/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>

*/
package cmd
import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var (
	output string
)

// wgetCmd represents the wget command
var wgetCmd = &cobra.Command{
	Use:     "wget",
	Example: "cobra wget https://www.baidu.com -o ./baidu.html",
	Args:    cobra.ExactArgs(1),
	Short:   "wget is a download cli.",
	Long:    `use wget to download everything you want from net.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("---wget running---")
		Download(args[0], output)
	},
}

func init() {
	rootCmd.AddCommand(wgetCmd)
	// Here you will define your flags and configuration settings.
	wgetCmd.Flags().StringVarP(&output, "output", "o", "", "output file")
	wgetCmd.MarkFlagRequired("output")
}
func Download(url string, path string) {
	out, err := os.Create(path)
	check(err)
	defer out.Close()

	res, err := http.Get(url)
	check(err)
	defer res.Body.Close()

	_, err = io.Copy(out, res.Body)
	check(err)
	fmt.Println("save as" + path)
}
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}