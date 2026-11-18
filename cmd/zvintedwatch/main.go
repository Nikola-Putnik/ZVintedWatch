// Command zvintedwatch is the ZVintedWatch entry point.
//
// It runs the monitoring engine, the Telegram notifier and the web UI in a
// single process. See the README for usage.
package main

import (
	"fmt"
	"os"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/version"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version.String())
		return
	}
	fmt.Println(version.String())
	fmt.Println("The server is not implemented yet.")
}
