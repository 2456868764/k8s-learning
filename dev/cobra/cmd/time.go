/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var echoTimes int

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
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println("[step_2] PreRun:")
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("[step_3] Run:")
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
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// timeCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// timeCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
