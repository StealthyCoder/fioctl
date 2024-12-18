package keys

import (
	"errors"
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/foundriesio/fioctl/client"
	"github.com/foundriesio/fioctl/subcommands"
)

func init() {
	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply staged TUF root updates for the Factory",
		Run:   doTufUpdatesApply,
	}
	applyCmd.Flags().StringP("txid", "x", "", "TUF root updates transaction ID.")
	tufUpdatesCmd.AddCommand(applyCmd)
}

func doTufUpdatesApply(cmd *cobra.Command, args []string) {
	factory := viper.GetString("factory")
	txid, _ := cmd.Flags().GetString("txid")

	// Clear the shortcut flag; this function will print the correct message on error.
	isTufUpdatesShortcut = false

	err := api.TufRootUpdatesApply(factory, txid)
	if err != nil {
		msg := "Failed to apply staged TUF root updates:\n%w\n"
		var isNonFatal bool
		if herr := client.AsHttpError(err); herr != nil {
			if herr.Response.StatusCode == 404 {
				// Double check: if there are no TUF updates - fail clean; otherwise, fatal error.
				updates, err1 := api.TufRootUpdatesGet(factory)
				if err1 == nil && updates.Status == client.TufRootUpdatesStatusNone {
					subcommands.DieNotNil(errors.New("There are no TUF root updates in progress."))
				}
			}
			isNonFatal = slices.Contains([]int{400, 401, 403, 422, 423}, herr.Response.StatusCode)
		}
		if isNonFatal {
			msg += `No changes were made to your Factory.
There are two options available for you now:
- fix the errors listed above and run the "fioctl keys tuf updates apply" again.
- cancel the staged TUF root updates using the "fioctl keys tuf updates cancel"`
		} else {
			msg += `
This is a critical error: Staged TUF root updates may be only partially applied to your Factory.
Please re-run the "fioctl keys tuf updates apply" soon after a short pause.
If the error persists, please contact customer support.`
		}
		err = fmt.Errorf(msg, err)
	}
	subcommands.DieNotNil(err)

	fmt.Println(`The staged TUF root updates were applied to your Factory.
Please, make sure that the updated TUF keys file(s) are stored in a safe place.`)
}
