/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"os"
	"path"

	"github.com/jackzampolin/cosmos-registrar/pkg/gitwrap"
	"github.com/jackzampolin/cosmos-registrar/pkg/node"
	"github.com/noandrea/go-codeowners"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

const (
	codeownersFile = "CODEOWNERS"
)

// claimCmd represents the claim command
var claimCmd = &cobra.Command{
	Use:   "claim",
	Short: "Claim a name for a cosmos based chain",
	Long: `This command allows you to submit a claim request for 
a name for you chain.

`,
	Run:  claim,
	Args: cobra.ExactArgs(1),
}

func init() {
	rootCmd.AddCommand(claimCmd)
}

func claim(cmd *cobra.Command, args []string) {
	// fetch network
	config.RPCAddr = strings.TrimSpace(args[0])
	// fetch the chain data
	claimName, err := node.FetchChainID(config)
	var (
		forkURL        = fmt.Sprintf("https://github.com/%s/%s.git", config.GitName, config.RegistryForkName)
		forkRepoFolder = path.Join(config.Workspace, config.RegistryForkName)
	)

	AbortIfError(err, "error fetching the chain ID: %v", err)
	fs := afero.NewOsFs()

	// check if root url is valid
	_, err = url.Parse(config.RegistryRoot)
	AbortIfError(err, "the registry root url is not a valid url: %s", config.RegistryRoot)

	repo, err := gitwrap.CloneOrOpen(forkURL, forkRepoFolder)
	AbortIfError(err, "aborted due to an error cloning the root repo: %v", err)

	gitwrap.PullBranch(repo, config.RegistryRootBranch)
	AbortIfError(err, "something went wrong checking out branch %s: %v", config.RegistryRootBranch, err)

	// now we have the root repo
	// read the the codeowners file
	co, err := codeowners.FromFile(forkRepoFolder)
	AbortIfError(err, "cannot find the CODEOWNERS file: %v", err)

	// see if there are already owners
	owners := co.LocalOwners(claimName)
	if owners != nil {
		currentUser, isOwner := fmt.Sprintf("@%s", config.GitName), false
		// owners already exists for path, check if the current user is among them
		for _, o := range owners {
			if o == currentUser {
				isOwner = true
				break
			}
		}
		if isOwner {
			println("you already successfully claimed the name %s, perhaps you want to update it?")
			return
		}
		// named owned by someone else
		println("the name", claimName, "is already claimed by someone else!")
		fmt.Printf("%#v", owners)
		return
	}
	// create the branch with the name `claimName`
	// TODO check if the branch exits
	println("checking out branch ", claimName)
	err = gitwrap.CreateBranch(repo, claimName)
	AbortIfError(err, "cannot create branch: %v", err)

	// add a subfolder `claimName`
	claimPath := path.Join(forkRepoFolder, claimName)
	err = fs.Mkdir(claimPath, 0700)
	println("creating chain folder for", claimName)
	AbortIfError(err, "cannot create claim folder: %v", err)

	// fetch the chain data
	err = node.DumpInfo(claimPath, claimName, config, logger)
	println("fetching chain data")
	AbortIfError(err, "error connecting to the node at %s: %v", config.RPCAddr, err)

	println("starting claiming process for", claimName)
	// add rule to the codeowner
	// TODO: ensure that the chain id is compliant to CAIP-2
	err = co.AddPattern(fmt.Sprintf("/%s/", claimName), []string{fmt.Sprint("@", config.GitName)})
	AbortIfError(err, "invalid claim name folder: %v", err)
	coFile := path.Join(forkRepoFolder, codeownersFile)
	err = co.ToFile(coFile)
	// commit the data

	err = gitwrap.StageToCommit(repo, codeownersFile, claimName)
	println("schedule changes for commit", codeownersFile)
	AbortIfError(err, "error adding the %s to git: %v", codeownersFile, err)

	commitMsg := fmt.Sprintf("submit record for chain ID %s", claimName)
	commit, err := gitwrap.CommitAndPush(repo,
		config.GitName,
		config.GitEmail,
		commitMsg,
		time.Now(),
		config.BasicAuth())
	AbortIfError(err, "git push error : %v", err)
	println("committed with hash", commit)

	// open the github page to submit the PR to mainRepo
	prURL := fmt.Sprintf("%s/compare/%s...%s:%s", config.RegistryRoot, config.RegistryRootBranch, config.GitName, claimName)
	println("the changes has been recorded in your private fork, to submit it create a pull request using this link")
	println(prURL)
	return
}

// AbortIfError abort command if there is an error
func AbortIfError(err error, message string, v ...interface{}) {
	if err == nil {
		return
	}
	fmt.Printf(message, v...)
	fmt.Println()
	os.Exit(1)
}
