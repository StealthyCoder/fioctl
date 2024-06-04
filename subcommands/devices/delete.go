package devices

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	cmd.AddCommand(&cobra.Command{
		Use:   "delete",
		Short: "Delete a device(s) registered to a factory.",
		Run:   doDelete,
		Args:  cobra.MinimumNArgs(1),
	})
}

func doDelete(cmd *cobra.Command, args []string) {
	factory := viper.GetString("factory")
	logrus.Debug("Deleting %r", args)

	for _, name := range args {
		fmt.Printf("Deleting %s .. ", name)
		d := api.DeviceApiByName(factory, name)
		if err := d.Delete(); err != nil {
			fmt.Printf("failed\n%s", err)
			os.Exit(1)
		} else {
			fmt.Printf("ok\n")
		}
	}
}
