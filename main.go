package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/DENICeG/go-rriclient/internal/env"
	"github.com/DENICeG/go-rriclient/pkg/rri"

	"github.com/sbreitf1/go-console"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	gitCommit string
	buildTime string
)

var (
	app            = kingpin.New("rri-client", "Client application for RRI")
	argAddress     = app.Arg("address", "Address and port like host:1234 of the RRI host").String()
	argFile        = app.Flag("file", "Input file containing RRI requests separated by a '=-=' line").Short('f').String()
	argVerbose     = app.Flag("verbose", "Print all sent and received requests").Short('v').Bool()
	argUser        = app.Flag("user", "RRI user to use for login").Short('u').String()
	argPassword    = app.Flag("pass", "RRI password to use for login. Will be asked for if only user is set").Short('p').String()
	argEnvironment = app.Flag("env", "Named environment to use or create").Short('e').String()
)

type environment struct {
	Address  string `json:"address"`
	User     string `json:"user"`
	Password string `json:"pass" jcrypt:"aes"`
}

func (e environment) HasCredentials() bool {
	return len(e.User) > 0 && len(e.Password) > 0
}

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))

	if err := func() error {
		env, err := retrieveEnvironment()
		if err != nil {
			return err
		}

		if len(env.Address) == 0 {
			return fmt.Errorf("missing RRI server address")
		}

		client, err := rri.NewClient(env.Address)
		if err != nil {
			return err
		}
		defer client.Close()
		if *argVerbose {
			client.RawQueryPrinter = rawQueryPrinter
		}

		if env.HasCredentials() {
			if err := client.Login(env.User, env.Password); err != nil {
				return err
			}
		}

		if len(*argFile) > 0 {
			content, err := ioutil.ReadFile(*argFile)
			if err != nil {
				return err
			}

			queries, err := rri.ParseQueries(string(content))
			if err != nil {
				return err
			}

			for _, query := range queries {
				console.Println("Exec query", query)
				response, err := client.SendQuery(query)
				if err != nil {
					return err
				}
				if response != nil && !response.IsSuccessful() {
					console.Printlnf("Query failed: %s", response.ErrorMsg())
					break
				}
			}

		} else {
			return runCLE(client)
		}

		return nil
	}(); err != nil {
		console.Printlnf("FATAL: %s", err.Error())
		os.Exit(1)
	}
}

func retrieveEnvironment() (environment, error) {
	envReader, err := env.NewReader(".rri-client")
	if err != nil {
		return environment{}, err
	}
	envReader.EnterEnvHandler = enterEnvironment
	envReader.GetEnvFileTitle = getEnvTitle

	var env environment
	if len(*argEnvironment) > 0 {
		err = envReader.CreateOrReadEnvironment(*argEnvironment, &env)
	} else if len(*argAddress) == 0 {
		err = envReader.SelectEnvironment(&env)
	}
	if err != nil {
		return environment{}, err
	}

	if len(*argAddress) > 0 {
		env.Address = *argAddress
	}
	if len(*argUser) > 0 {
		env.User = *argUser
	}
	if len(*argPassword) > 0 {
		env.Password = *argPassword
	}

	if len(env.User) > 0 && len(env.Password) == 0 {
		var err error
		console.Printlnf("Please enter RRI password for user %q", env.User)
		console.Print("> ")
		env.Password, err = console.ReadPassword()
		if err != nil {
			return environment{}, err
		}
	}

	return env, nil
}

func enterEnvironment(envName string, env interface{}) error {
	e, ok := env.(*environment)
	if !ok {
		panic(fmt.Sprintf("environment has unexpected type %T", env))
	}

	var err error

	console.Print("Address (Host:Port)> ")
	e.Address, err = console.ReadLine()
	if err != nil {
		return err
	}

	console.Print("User> ")
	e.User, err = console.ReadLine()
	if err != nil {
		return err
	}

	console.Print("Password> ")
	e.Password, err = console.ReadPassword()
	if err != nil {
		return err
	}

	return nil
}

func getEnvTitle(envName, envFile string) string {
	data, err := ioutil.ReadFile(envFile)
	if err != nil {
		return envName
	}

	type envPreview struct {
		Address string `json:"address"`
		User    string `json:"user"`
	}
	var env envPreview
	if err := json.Unmarshal(data, &env); err != nil {
		return envName
	}

	var suffix string
	if len(env.User) > 0 {
		suffix = fmt.Sprintf(" (%s@%s)", env.User, env.Address)
	} else {
		suffix = fmt.Sprintf(" (%s)", env.Address)
	}
	return fmt.Sprintf("%s%s", envName, suffix)
}
