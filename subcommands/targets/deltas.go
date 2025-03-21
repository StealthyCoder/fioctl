package targets

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/foundriesio/fioctl/client"
	"github.com/foundriesio/fioctl/subcommands"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	byTag  string
	dryRun bool
	hwId   string
)

func init() {
	var deltas = &cobra.Command{
		Use:   "static-deltas <target-version> [<from-version>...]",
		Short: "Generate static deltas to the given Target version to make OTAs faster",
		Run:   doDeltas,
		Args:  cobra.MinimumNArgs(1),
		Long: `In many cases OTA updates will have multiple OSTree changes. These updates
can be downloaded faster by generating OSTree static
deltas. Static deltas are generated using "from(sha) -> to(sha)" type
logic. This command takes the given Target version, and produces a
number of static deltas to ensure devices are updated efficiently.`,
		Example: `
  # There are two ways to run this command:

  # Generate static deltas for 30->42 and 31->42
  fioctl targets static-deltas 42 30 31

  # Find the target versions of all devices configured to the "prod" tag.
  # Generate a static delta from those versions to version 42.
  fioctl targets static-deltas --by-tag prod 42`,
	}
	cmd.AddCommand(deltas)
	deltas.Flags().StringVarP(&byTag, "by-tag", "", "", "Find from-versions devices on the given tag")
	deltas.Flags().BoolVarP(&noTail, "no-tail", "", false, "Don't tail output of CI Job")
	deltas.Flags().BoolVarP(&dryRun, "dryrun", "", false, "Only show what deltas would be produced")
	deltas.Flags().StringVarP(&hwId, "hw-id", "", "", "Filter from and to Targets by the given hardware ID")
}

func findVersions(maxVer int, forTag string, tags []client.TagStatus) (bool, []int) {
	var versions []int
	for _, status := range tags {
		if status.Name == forTag {
			for _, t := range status.Targets {
				if !t.IsOrphan && t.Version < maxVer {
					versions = append(versions, t.Version)
				}
			}
			return true, versions
		}
	}
	return false, nil
}

func doDeltas(cmd *cobra.Command, args []string) {
	factory := viper.GetString("factory")
	toVer, err := strconv.Atoi(args[0])
	subcommands.DieNotNil(err)
	if len(hwId) > 0 {
		logrus.Debugf("Generating static deltas to Target %d with hardware ID %s in Factory %s", toVer, hwId, factory)
	} else {
		logrus.Debugf("Generating static deltas to Target %d in Factory %s", toVer, factory)
	}

	var froms []int
	for _, fromStr := range args[1:] {
		fromI, err := strconv.Atoi(fromStr)
		subcommands.DieNotNil(err)
		if fromI >= toVer {
			subcommands.DieNotNil(fmt.Errorf("from-version %d is newer than to-version %d", fromI, toVer))
		}
		froms = append(froms, fromI)
	}
	if len(byTag) > 0 {
		status, err := api.FactoryStatus(factory, 4)
		subcommands.DieNotNil(err)

		foundCi, versionsCi := findVersions(toVer, byTag, status.Tags)
		foundProd, versionsProd := findVersions(toVer, byTag, status.ProdTags)
		if !foundCi && !foundProd {
			subcommands.DieNotNil(fmt.Errorf("No tags named '%s' found", byTag))
		}
		if foundCi {
			froms = append(froms, versionsCi...)
		}
		if foundProd {
			froms = append(froms, versionsProd...)
		}
	}
	if len(froms) == 0 {
		subcommands.DieNotNil(errors.New("No Targets found to generate deltas for."))
	}
	if dryRun {
		if len(hwId) > 0 {
			fmt.Printf("Dry run: Would generate static deltas for Targets with hardware ID %s and versions:\n", hwId)
		} else {
			fmt.Println("Dry run: Would generate static deltas for Target versions:")
		}
		for _, v := range froms {
			fmt.Println("  ", v, "->", toVer)
		}
		return
	}
	logrus.Debugf("Froms: %v", froms)

	jobServUrl, webUrl, err := api.TargetDeltasCreate(factory, toVer, froms, hwId)
	subcommands.DieNotNil(err)
	fmt.Printf("CI URL: %s\n", webUrl)
	if !noTail {
		api.JobservTail(jobServUrl)
	}
}
