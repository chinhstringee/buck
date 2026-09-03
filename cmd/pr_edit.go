package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/chinhstringee/buck/internal/bitbucket"
	"github.com/spf13/cobra"
)

type prEditOptions struct {
	selector    string
	title       string
	titleEdited bool
	body        string
	bodyEdited  bool
	out         io.Writer
}

var prEditCmd = newPREditCmd(runPREdit)

func newPREditCmd(runF func(*prEditOptions) error) *cobra.Command {
	opts := &prEditOptions{}
	var bodyFile string

	cmd := &cobra.Command{
		Use:   "edit [<number> | <branch>]",
		Short: "Edit a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.selector = args[0]
			}

			opts.titleEdited = cmd.Flags().Changed("title")
			bodyProvided := cmd.Flags().Changed("body")
			bodyFileProvided := cmd.Flags().Changed("body-file")
			if bodyProvided && bodyFileProvided {
				return fmt.Errorf("specify only one of --body or --body-file")
			}

			opts.bodyEdited = bodyProvided || bodyFileProvided
			if bodyFileProvided {
				var data []byte
				var err error
				if bodyFile == "-" {
					data, err = io.ReadAll(cmd.InOrStdin())
				} else {
					data, err = os.ReadFile(bodyFile)
				}
				if err != nil {
					return fmt.Errorf("read body file: %w", err)
				}
				opts.body = string(data)
			}

			if !opts.titleEdited && !opts.bodyEdited {
				return fmt.Errorf("--title or --body required when not running interactively")
			}

			opts.out = cmd.OutOrStdout()
			return runF(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.title, "title", "t", "", "Set the new title")
	cmd.Flags().StringVarP(&opts.body, "body", "b", "", "Set the new body")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from file (use \"-\" for standard input)")

	return cmd
}

func runPREdit(opts *prEditOptions) error {
	prID, parseErr := strconv.Atoi(opts.selector)
	byID := parseErr == nil
	if byID && prID < 1 {
		return fmt.Errorf("invalid pull request number %q", opts.selector)
	}

	contextSelector := opts.selector
	if byID && prFlagRepos == "" && prFlagGroup == "" && !prFlagInteractive {
		contextSelector = ""
	}
	ctx, err := resolvePRContext(contextSelector)
	if err != nil {
		return err
	}
	if len(ctx.repos) != 1 {
		return fmt.Errorf("pr edit requires exactly one repository, got %d", len(ctx.repos))
	}

	repo := ctx.repos[0]
	if !byID {
		pr, err := ctx.client.FindPRByBranch(ctx.workspace, repo, ctx.branchName, "OPEN")
		if err != nil {
			return err
		}
		prID = pr.ID
	}

	request := bitbucket.PRUpdateRequest{}
	fields := make([]string, 0, 2)
	if opts.titleEdited {
		request.Title = &opts.title
		fields = append(fields, "title")
	}
	if opts.bodyEdited {
		request.Description = &opts.body
		fields = append(fields, "body")
	}

	if prFlagDryRun {
		fmt.Fprintf(opts.out, "Dry run: would edit PR #%d in %s/%s (%s)\n",
			prID, ctx.workspace, repo, strings.Join(fields, ", "))
		return nil
	}

	updated, err := ctx.client.UpdatePR(ctx.workspace, repo, prID, request)
	if err != nil {
		return fmt.Errorf("failed to edit PR #%d: %w", prID, err)
	}
	fmt.Fprintln(opts.out, updated.Links.HTML.Href)
	return nil
}

func init() {
	prCmd.AddCommand(prEditCmd)
}
