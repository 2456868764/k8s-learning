/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"strings"
)


// I'm declaring as vars so I can test easier, I recommend declaring these as constants
var (
	// The name of our config file, without the file extension because viper supports many different config file languages.
	defaultConfigFilename = "stingoftheviper"

	// The environment variable prefix of all environment variables bound to our command line flags.
	// For example, --number is bound to DUBBO_NUMBER.
	envPrefix = "DUBBO"

	// Replace hyphenated flag names with camelCase in the config file
	replaceHyphenWithCamelCase = false
)


var echoTimes int
var echoSeconds int
// timeCmd represents the time command
var timeCmd = &cobra.Command{
	Use:   "time",
	Short: "Echo anything to the screen more time",
	Long: `echo things multiple times back to the user by providing
a count and a string
`,
    Example: "cobra time -t 3 hello",
    Args: cobra.MinimumNArgs(1),
    PersistentPreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("[step_1] PersistentPreRun:")
		initialize(cmd)
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("[step_2] PreRun:")
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("[step_3] Run:")
		fmt.Printf("echoTime:%d, echoSeceond:%d\n", echoTimes, echoSeconds)
		for i:=0; i<echoTimes;i++ {
			fmt.Println("Echo:" + strings.Join(args, " "))
		}
	},

	PostRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("[step_4] PostRun:")
	},

	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("[step_5] PersistentPostRun:")
	},
}

func init() {

	rootCmd.AddCommand(timeCmd)
	timeCmd.Flags().IntVarP(&echoTimes, "times", "t", 1, "times to echo input")
	timeCmd.Flags().IntVarP(&echoSeconds, "second-slash", "s", 0, "times to echo second")


	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// timeCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// timeCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}



func initialize(cmd *cobra.Command) error {
	v := viper.New()

	// When we bind flags to environment variables expect that the
	// environment variables are prefixed, e.g. a flag like --number
	// binds to an environment variable DUBBO_NUMBER. This helps
	// avoid conflicts.
	v.SetEnvPrefix(envPrefix)

	// Environment variables can't have dashes in them, so bind them to their equivalent
	// keys with underscores, e.g. --favorite-color to DUBBO_FAVORITE_COLOR
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Bind to environment variables
	// Works great for simple config names, but needs help for names
	// like --favorite-color which we fix in the bindFlags function
	v.AutomaticEnv()

	// Bind the current command's flags to viper
	bindFlags(cmd, v)

	return nil
}

// Bind each cobra flag to its associated viper configuration (config file and environment variable)
func bindFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Determine the naming convention of the flags when represented in the config file
		configName := f.Name
		// If using camelCase in the config file, replace hyphens with a camelCased string.
		// Since viper does case-insensitive comparisons, we don't need to bother fixing the case, and only need to remove the hyphens.
		if replaceHyphenWithCamelCase {
			configName = strings.ReplaceAll(f.Name, "-", "")
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(configName) {
			val := v.Get(configName)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
}

