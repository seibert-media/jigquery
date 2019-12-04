package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/seibert-media/jigquery"

	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	mode          = flag.String("mode", "", "the mode to run in [generate, deploy, schema]")
	help          = flag.Bool("help", false, "show this usage info")
	debug         = flag.Bool("debug", false, "print debug logging")
	googleProject = flag.String("googleProject", os.Getenv("GOOGLE_CLOUD_PROJECT"), "the google cloud project to use")
	interactive   = flag.Bool("i", false, "run in interactive mode")
)

func main() {
	flag.Parse()

	localLogger, err := log.New("", true)
	if err != nil {
		panic(err)
	}
	if *debug {
		localLogger.SetLevel(zapcore.DebugLevel)
	}

	ctx := log.WithLogger(context.Background(), localLogger)

	if len(*googleProject) < 1 {
		fmt.Println("missing google project")
		os.Exit(1)
	}

	if *help {
		flag.Usage()
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		fmt.Println("working directory:", wd)
		os.Exit(0)
	}

	if len(*mode) > 0 {
		switch *mode {
		case "generate":
			GenerateSecret(ctx, *googleProject)
		case "deploy":
			if err := Deploy(ctx, *googleProject); err != nil {
				log.From(ctx).Fatal("deploying", zap.Error(err))
			}
		case "schema":
			if _, err := uploadSchema(ctx); err != nil {
				log.From(ctx).Fatal("deploying", zap.Error(err))
			}
		default:
			fmt.Printf("%s\n 	-mode generate 	// Generate .env and .env.yaml files from the Jira auth.json under the provided path\n", path.Base(os.Args[0]))
			fmt.Printf("	-mode deploy 	// deploy the function and it's related resources\n")
			fmt.Printf("	-mode schema 	// update the schema\n")
		}
		os.Exit(0)
	}

	if err := jigquery.InsertIssues(ctx, jigquery.PubSubMessage{}); err != nil {
		os.Exit(1)
	}
}
