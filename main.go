package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/urfave/cli"
)

var VERSION = "dev"
var RANCHERVERSION = "v1.6.6"

func main() {
	app := cli.NewApp()
	app.Name = "infra-version"
	app.Usage = "Tool for querying infra services version given rancher version"
	app.Before = func(ctx *cli.Context) error {
		if ctx.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	app.Version = VERSION
	app.Author = "lawr"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Debug logging",
		},
		cli.StringFlag{
			Name:  "branch",
			Usage: "specific rancher catalog branch",
		},
	}
	app.Usage = "infra-version [options] RancherVersion"
	app.Action = getTemplates

	args := os.Args
	if len(args) != 2 {
		logrus.Fatal("mismatch paras")
	}
	fmt.Printf("searching infra services of Rancher version %s\n", args[1])
	RANCHERVERSION = args[1]

	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(err)
	}
}
