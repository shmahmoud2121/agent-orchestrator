package cli

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type repoTrustScan struct {
	Fingerprint string `json:"fingerprint"`
	Approved    bool   `json:"approved"`
	Files       []struct {
		Path    string   `json:"path"`
		Summary []string `json:"summary"`
	} `json:"files"`
}

func newProjectTrustCommand(ctx *commandContext) *cobra.Command {
	var yes, revoke bool
	var branch string
	cmd := &cobra.Command{
		Use: "trust <id>", Short: "Review and approve repository agent configuration",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return usageError{err}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(os.Getenv("AO_SESSION_ID")) != "" {
				return errors.New("REPO_TRUST_FORBIDDEN: Only the person can change repository trust. Ask them in your report.") //nolint:revive,staticcheck // shown verbatim to the agent
			}
			path := "projects/" + url.PathEscape(args[0]) + "/repo-trust"
			var scan repoTrustScan
			if err := ctx.getJSON(cmd.Context(), path+"?branch="+url.QueryEscape(branch), &scan); err != nil {
				return err
			}
			for _, f := range scan.Files {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%q\n", f.Path); err != nil {
					return err
				}
				for _, line := range f.Summary {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  "+line); err != nil {
						return err
					}
				}
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Fingerprint:", scan.Fingerprint); err != nil {
				return err
			}
			if !revoke && len(scan.Files) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "No repository agent configuration requires approval."); err != nil {
					return err
				}
				return nil
			}
			if !yes {
				action := "Approve execution of this repository configuration? [y/N] "
				if revoke {
					action = "Revoke repository configuration approval? [y/N] "
				}
				if _, err := fmt.Fprint(cmd.OutOrStdout(), action); err != nil {
					return err
				}
				answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if err != nil && answer == "" {
					return fmt.Errorf("confirmation required (or pass --yes): %w", err)
				}
				if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Cancelled."); err != nil {
						return err
					}
					return nil
				}
			}
			if revoke {
				if err := ctx.deleteJSON(cmd.Context(), path, &scan); err != nil {
					return err
				}
			} else {
				in := struct {
					Fingerprint string `json:"fingerprint"`
					Branch      string `json:"branch,omitempty"`
				}{scan.Fingerprint, branch}
				if err := ctx.postJSON(cmd.Context(), path, in, &scan); err != nil {
					return err
				}
			}
			if revoke {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Repository trust revoked."); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Repository configuration approved."); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "", "Also review configuration shipped by this worker branch")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm after printing the detected files")
	cmd.Flags().BoolVar(&revoke, "revoke", false, "Revoke the project's repository configuration approval")
	return cmd
}
