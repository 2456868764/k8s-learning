/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)



// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "cobra",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,

	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("ROOT [step_1] PersistentPreRun:")
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("ROOT [step_2] PreRun:")
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("ROOT [step_3] Run:")

	},

	PostRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("ROOT [step_4] PostRun:")
	},

	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("ROOT [step_5] PersistentPostRun:")
	},

	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var (
	Verbose bool
	CfgFile string

)
func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&CfgFile, "config", "c", "",  "config file (default is $HOME/.cobra.yaml)")
	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

