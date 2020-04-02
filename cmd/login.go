package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/foundriesio/fioctl/client"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Access Foundries.io services with your client credentials",
	Run:   doLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func assertLogin(cmd *cobra.Command, args []string) {
	initViper(cmd, args)
	if len(config.Token) > 0 {
		return
	}

	if len(config.ClientCredentials.ClientId) == 0 {
		fmt.Println("ERROR: Please run: \"fioctl login\" first")
		os.Exit(1)
	}
	creds := client.NewClientCredentials(config.ClientCredentials)

	expired, err := creds.IsExpired()
	if err != nil {
		fmt.Println("ERROR: ", err)
		os.Exit(1)
	}

	if !expired && len(creds.Config.AccessToken) > 0 {
		return
	}

	if len(creds.Config.AccessToken) == 0 {
		if err := creds.Get(); err != nil {
			fmt.Println("ERROR: ", err)
			os.Exit(1)
		}
	} else if creds.HasRefreshToken() {
		if err := creds.Refresh(); err != nil {
			fmt.Println("ERROR: ", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("ERROR: Missing refresh token")
		os.Exit(1)
	}
	saveCreds(creds.Config)
	api = client.NewApiClient("https://api.foundries.io", config, "")
}

func doLogin(cmd *cobra.Command, args []string) {
	logrus.Debug("Executing login command")

	creds := client.NewClientCredentials(config.ClientCredentials)
	if creds.Config.ClientId == "" || creds.Config.ClientSecret == "" {
		creds.Config.ClientId, creds.Config.ClientSecret = promptForCreds()
		saveCreds(creds.Config)
	}

	if creds.Config.ClientId == "" || creds.Config.ClientSecret == "" {
		fmt.Println("Cannot execute login without client ID or client secret.")
		os.Exit(1)
	}

	expired, err := creds.IsExpired()
	if err != nil {
		fmt.Println("ERROR: ", err)
		os.Exit(1)
	}

	if expired && creds.HasRefreshToken() {
		if err := creds.Refresh(); err != nil {
			fmt.Println("ERROR: ", err)
			os.Exit(1)
		}
	} else if creds.Config.AccessToken == "" {
		if err := creds.Get(); err != nil {
			fmt.Println("ERROR: ", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("You are already logged in to Foundries.io services.")
		os.Exit(0)
	}

	saveCreds(creds.Config)
	fmt.Println("You are now logged in to Foundries.io services.")
}

func saveCreds(c client.OAuthConfig) {
	viper.Set("clientcredentials.client_id", c.ClientId)
	viper.Set("clientcredentials.client_secret", c.ClientSecret)

	viper.Set("clientcredentials.access_token", c.AccessToken)
	viper.Set("clientcredentials.refresh_token", c.RefreshToken)
	viper.Set("clientcredentials.token_type", c.TokenType)
	viper.Set("clientcredentials.expires_in", c.ExpiresIn)
	viper.Set("clientcredentials.created", c.Created)

	if err := viper.WriteConfig(); err != nil {
		fmt.Println("ERROR: ", err)
		os.Exit(1)
	}
}

func promptForCreds() (string, string) {
	logrus.Debug("Reading client ID/secret from stdin")

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Please visit:\n\n")
	fmt.Print("  https://app.foundries.io/settings/tokens/\n\n")
	fmt.Print("and create a new \"Application Credential\" to provide inputs below.\n\n")
	fmt.Print("Client ID: ")
	scanner.Scan()
	clientId := strings.Trim(scanner.Text(), " ")

	fmt.Print("Client secret: ")
	scanner.Scan()
	clientSecret := strings.Trim(scanner.Text(), " ")

	if clientId == "" || clientSecret == "" {
		fmt.Println("Client ID and client credentials are both required.")
		os.Exit(1)
	}

	return clientId, clientSecret
}
