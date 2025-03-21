package waves

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go/canonical/json"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/slices"

	"github.com/foundriesio/fioctl/client"
	"github.com/foundriesio/fioctl/subcommands"
	"github.com/foundriesio/fioctl/subcommands/keys"
	tuf "github.com/theupdateframework/notary/tuf/data"
)

func init() {
	initCmd := &cobra.Command{
		Use:   "init <wave> <version> <tag>",
		Short: "Create a new Wave from Targets of a given version",
		Long: `Create a new Wave from Targets of a given version.

This command only initializes a Wave. It does not provision updates to devices.
Use "fioctl wave rollout <wave> <group>" to trigger updates of this Wave to a device group.
Use "fioctl wave complete <wave>" to update all devices (make it globally available).
Use "fioctl wave cancel <wave> to cancel a Wave (make it no longer available).

We recommend that you generate static deltas for your production Targets to optimize OTA update downloads. 
Consider generating a static delta for Targets using:
$ fioctl targets static-deltas
`,
		Run:  doInitWave,
		Args: cobra.ExactArgs(3),
		Example: `
Start a new Wave for the Target version 4 and the 'production' device tag:
$ fioctl wave init -k ~/path/to/keys/targets.only.key.tgz wave-name 4 production

Start a new Wave for the Target version 16 and also prune old production versions 1,2,3 and 4 in this case:
$ fioctl wave init -k ~/path/to/keys/targets.only.key.tgz wave-name 16 production --prune 1,2,3,4

`,
	}
	cmd.AddCommand(initCmd)
	initCmd.Flags().IntP("expires-days", "e", 0, `Role expiration in days; default 365.
The same expiration will be used for production Targets when a Wave is complete.`)
	initCmd.Flags().StringP("expires-at", "E", "", `Role expiration date and time in RFC 3339 format.
The same expiration will be used for production Targets when a Wave is complete.
When set this value overrides an 'expires-days' argument.
Example: 2020-01-01T00:00:00Z`)
	initCmd.Flags().BoolP("dry-run", "d", false, "Don't create a Wave, print it to standard output.")
	initCmd.Flags().StringSlice("prune", []string{}, `Prune old unused Target(s) from the production metadata.
Example: 1,2,3`)
	initCmd.Flags().StringP("keys", "k", "", "Path to <offline-creds.tgz> used to sign Wave Targets.")
	initCmd.Flags().StringP("source-tag", "", "", "Match this tag when looking for Target versions. Certain advanced tagging configurations may require this argument.")
	_ = initCmd.MarkFlagRequired("keys")
}

func doInitWave(cmd *cobra.Command, args []string) {
	factory := viper.GetString("factory")
	name, version, tag := args[0], args[1], args[2]
	intVersion, err := strconv.ParseInt(version, 10, 32)
	subcommands.DieNotNil(err, "Version must be an integer")
	expires := readExpiration(cmd)
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	prune, _ := cmd.Flags().GetStringSlice("prune")
	sourceTag, _ := cmd.Flags().GetString("source-tag")
	offlineKeys := readOfflineKeys(cmd)

	logrus.Debugf("Creating a Wave %s for Tactory %s Targets version %s and new tag %s expires %s",
		name, factory, version, tag, expires.Format(time.RFC3339))

	new_targets, err := api.TargetsList(factory, version)
	subcommands.DieNotNil(err)
	if len(new_targets) == 0 {
		subcommands.DieNotNil(fmt.Errorf("No targets found for version %s", version))
	}

	current_targets, err := api.ProdTargetsGet(factory, tag, false)
	subcommands.DieNotNil(err)

	targets := client.AtsTargetsMeta{}
	targets.Type = tuf.TUFTypes["targets"]
	targets.Expires = expires
	targets.Version = int(intVersion)
	if current_targets == nil {
		targets.Targets = make(tuf.Files)
	} else {
		targets.Targets = current_targets.Signed.Targets
		if targets.Version <= current_targets.Signed.Version {
			subcommands.DieNotNil(fmt.Errorf(
				"Cannot create a Wave for a version lower than or equal to production Targets for the same tag"))
		}
	}

	for name, file := range new_targets {
		if _, exists := targets.Targets[name]; exists {
			subcommands.DieNotNil(fmt.Errorf("Target %s already exists in production Targets for tag %s", name, tag))
		}

		if len(sourceTag) > 0 {
			custom, err := api.TargetCustom(file)
			subcommands.DieNotNil(err)
			found := false
			for _, tag := range custom.Tags {
				if tag == sourceTag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		subcommands.DieNotNil(replaceTags(&file, tag), fmt.Sprintf("Malformed CI Target custom field %s", name))
		targets.Targets[name] = file
	}

	if len(prune) > 0 {
		if slices.Contains(prune, version) {
			subcommands.DieNotNil(fmt.Errorf("Cannot prune current version"))
		}
		targets = pruneTargets(&targets, prune)
	}

	meta, err := json.MarshalCanonical(targets)
	subcommands.DieNotNil(err, "Failed to serialize new Targets")
	signatures := signTargets(meta, factory, offlineKeys)

	signed := tuf.Signed{
		// Existing signatures are invalidated by new targets, throw them away.
		Signatures: signatures,
		Signed:     &json.RawMessage{},
	}
	_ = signed.Signed.UnmarshalJSON(meta)

	wave := client.WaveCreate{
		Name:    name,
		Version: version,
		Tag:     tag,
		Targets: signed,
	}
	if dryRun {
		payload, err := subcommands.MarshalIndent(&wave, "", "  ")
		subcommands.DieNotNil(err, "Failed to marshal a Wave")
		fmt.Println(string(payload))
	} else {
		subcommands.DieNotNil(api.FactoryCreateWave(factory, &wave), "Failed to create a Wave")
	}

	if !hasStaticDelta(new_targets) {

		fmt.Print(`
WARNING: You created a Wave for a Target version without static deltas.

We recommend that you generate static deltas for your production Targets to optimize OTA update downloads. 
Consider generating a static delta for Targets using: 
$ fioctl targets static-deltas

You can then cancel this Wave and create a new one for Target with a static delta.
`)

	}
}

func hasStaticDelta(new_targets tuf.Files) bool {
	for _, file := range new_targets {
		custom, err := api.TargetCustom(file)
		subcommands.DieNotNil(err)
		if custom.DeltaStats == nil {
			return false
		}
	}
	return true
}

func pruneTargets(currentTargets *client.AtsTargetsMeta, versions []string) client.AtsTargetsMeta {
	var missing []string
	for _, version := range versions {
		found := false
		for name, file := range currentTargets.Targets {
			custom, err := api.TargetCustom(file)
			subcommands.DieNotNil(err)
			if custom.Version == version {
				delete(currentTargets.Targets, name)
				found = true
			}
		}
		if !found {
			missing = append(missing, version)
		}
	}
	if len(missing) > 0 {
		subcommands.DieNotNil(fmt.Errorf(""), fmt.Sprintf("Unable to prune the following versions: %s", strings.Join(missing, ",")))
	}

	return *currentTargets
}

func signTargets(meta []byte, factory string, offlineKeys keys.OfflineCreds) []tuf.Signature {
	root, err := api.TufRootGet(factory)
	subcommands.DieNotNil(err, "Failed to fetch root role")
	onlinePub, err := api.TufTargetsOnlineKey(factory)
	subcommands.DieNotNil(err, "Failed to fetch online Targets public key")

	targetsKids := root.Signed.Roles["targets"].KeyIDs
	signerKids := make([]string, 0, len(targetsKids)-1)
	for _, kid := range targetsKids {
		pub := root.Signed.Keys[kid].KeyValue.Public
		if pub == onlinePub.KeyValue.Public {
			continue
		}
		signerKids = append(signerKids, kid)
	}

	if len(signerKids) == 0 {
		subcommands.DieNotNil(fmt.Errorf(`Root role is not configured to sign Targets offline.
Please, run "fioctl keys tuf rotate-offline-key --role=targets" in order to create offline Targets keys.`))
	}

	signer, err := keys.FindOneTufSigner(root, offlineKeys, signerKids)
	subcommands.DieNotNil(err, keys.ErrMsgReadingTufKey("targets", "current"))
	signatures, err := keys.SignTufMeta(meta, signer)
	subcommands.DieNotNil(err, "Failed to sign new targets")
	return signatures
}

func readExpiration(cmd *cobra.Command) (expires time.Time) {
	var err error
	if cmd.Flags().Changed("expires-at") {
		at, _ := cmd.Flags().GetString("expires-at")
		expires, err = time.Parse(time.RFC3339, at)
		subcommands.DieNotNil(err, "Invalid expires-at value:")
		expires = expires.UTC()
	} else {
		days := 365
		if cmd.Flags().Changed("expires-days") {
			days, _ = cmd.Flags().GetInt("expires-days")
		}
		expires = time.Now().UTC().Add(time.Duration(days*24) * time.Hour)
	}
	// This forces a JSON marshaller to use an RFC3339 instead of the default RFC3339Nano format.
	// An aktualizr we use on devices to update targets doesn't understand the latter one.
	return expires.Round(time.Second)
}

func readOfflineKeys(cmd *cobra.Command) keys.OfflineCreds {
	offlineKeysFile, _ := cmd.Flags().GetString("keys")
	offlineKeys, err := keys.GetOfflineCreds(offlineKeysFile)
	subcommands.DieNotNil(err, "Failed to open offline keys file")
	return offlineKeys
}

func replaceTags(target *tuf.FileMeta, tag string) error {
	// A client.TufCustom isn't suitable here, as a target might have other "non-standard" fields not
	// covered by this struct.  We still need to preserve all the original fields except for tags.
	var custom map[string]interface{}
	if err := json.Unmarshal(*target.Custom, &custom); err != nil {
		return err
	}
	// We don't care what tags are there, but we know what tags we want to be there
	custom["tags"] = []string{tag}
	if data, err := json.MarshalCanonical(custom); err != nil {
		return err
	} else {
		return target.Custom.UnmarshalJSON(data)
	}
}
