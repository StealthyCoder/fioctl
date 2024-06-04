package devices

import (
	"github.com/cheynewallace/tabby"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/foundriesio/fioctl/client"
	"github.com/foundriesio/fioctl/subcommands"
)

func init() {
	listUpdatesCmd := &cobra.Command{
		Use:    "list <device>",
		Short:  "[DEPRECATED] Please use: fioctl devices updates <update-id>",
		Hidden: true,
		Run:    doListUpdates,
		Args:   cobra.ExactArgs(1),
	}
	updatesCmd.AddCommand(listUpdatesCmd)
	listUpdatesCmd.Flags().IntVarP(&listLimit, "limit", "n", 0, "Limit the number of results displayed.")
}

func doListUpdates(cmd *cobra.Command, args []string) {
	factory := viper.GetString("factory")
	logrus.Debug("Showing device updates")
	t := tabby.New()
	t.AddHeader("ID", "TIME", "VERSION", "TARGET")
	d := api.DeviceApiByName(factory, args[0])
	var ul *client.UpdateList
	for {
		var err error
		if ul == nil {
			ul, err = d.ListUpdates()
		} else {
			if ul.Next != nil {
				ul, err = d.ListUpdatesCont(*ul.Next)
			} else {
				break
			}
		}
		subcommands.DieNotNil(err)
		for _, update := range ul.Updates {
			t.AddLine(update.CorrelationId, update.Time, update.Version, update.Target)
			listLimit -= 1
			if listLimit == 0 {
				break
			}
		}
		if listLimit == 0 {
			break
		}
	}
	t.Print()
}
