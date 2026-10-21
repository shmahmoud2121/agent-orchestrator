package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"

	"github.com/spf13/cobra"
)

// mergeSessionRequest mirrors controllers.MergeSessionRequest.
type mergeSessionRequest struct {
	DryRun          bool   `json:"dryRun,omitempty"`
	CallerSessionID string `json:"callerSessionId,omitempty"`
}

// mergeOverlap mirrors controllers.MergeOverlap.
type mergeOverlap struct {
	SessionID string   `json:"sessionId"`
	Branch    string   `json:"branch"`
	Files     []string `json:"files"`
}

// mergeSessionResponse mirrors controllers.MergeSessionResponse.
type mergeSessionResponse struct {
	SessionID         string         `json:"sessionId"`
	Branch            string         `json:"branch"`
	BaseBranch        string         `json:"baseBranch"`
	DryRun            bool           `json:"dryRun"`
	BaseBefore        string         `json:"baseBefore"`
	MergeCommit       string         `json:"mergeCommit,omitempty"`
	CommitCount       int            `json:"commitCount"`
	ChangedFiles      []string       `json:"changedFiles"`
	ChangedFilesTotal int            `json:"changedFilesTotal"`
	Verified          bool           `json:"verified"`
	UpdatedCheckout   string         `json:"updatedCheckout,omitempty"`
	Overlaps          []mergeOverlap `json:"overlaps"`
	Notified          []string       `json:"notified"`
	Warnings          []string       `json:"warnings"`
	PushAttempted     bool           `json:"pushAttempted"`
	Pushed            bool           `json:"pushed"`
	PushedRef         string         `json:"pushedRef,omitempty"`
	PushedCommit      string         `json:"pushedCommit,omitempty"`
	PushError         string         `json:"pushError,omitempty"`
}

// A base-moved retry can run the full verify command twice; leave time for
// the bounded push and best-effort notifications after landing.
const localMergeTimeout = 2*domain.MaxVerifyTimeoutSeconds*time.Second + 5*time.Minute

type mergeOptions struct {
	dryRun bool
	json   bool
}

func newMergeCommand(ctx *commandContext) *cobra.Command {
	var opts mergeOptions
	cmd := &cobra.Command{
		Use:   "merge <worker-session-id>",
		Short: "Merge a worker's branch into the project's base branch on this machine",
		Long: `Merge a worker session's committed branch into the project's base branch,
locally and without any remote. AO merges in a throwaway worktree, runs the
project's verify command on the result, and only then advances the base branch.
If the base branch is checked out (usually your main checkout), it must be
clean and is fast-forwarded so its files match.

The worker's worktree must be clean (everything committed). On a conflict
nothing lands; the command lists the conflicting files and the worker must
merge the base branch into its own branch, resolve, commit, and ask again.

Only the orchestrator (or a person) can merge: worker sessions are refused.
When the project's pushAfterMerge setting is on, AO pushes the base branch to
origin after the merge landed; a failed push never undoes the merge.

--dry-run merges and runs the project verify command in a throwaway worktree,
then discards it without advancing a branch, pushing, or messaging anyone.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return usageError{errors.New("usage: ao merge <worker-session-id> [--dry-run]")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			session := strings.TrimSpace(args[0])
			req := mergeSessionRequest{DryRun: opts.dryRun, CallerSessionID: strings.TrimSpace(os.Getenv("AO_SESSION_ID"))}
			var res mergeSessionResponse
			if err := ctx.doJSONPathWithHeadersAndTimeout(cmd.Context(), http.MethodPost, "/api/v1/sessions/"+url.PathEscape(session)+"/merge", req, &res, nil, localMergeTimeout); err != nil {
				return err
			}
			if opts.json {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			return printMergeResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Check the merge without moving any branch, pushing, or messaging anyone")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output the result as JSON")
	return cmd
}

func printMergeResult(out io.Writer, res mergeSessionResponse) error {
	var b strings.Builder
	if res.DryRun {
		fmt.Fprintf(&b, "dry run: %s (%s) merges cleanly into %s at %s: %d commit(s), %d file(s)\n",
			res.SessionID, res.Branch, res.BaseBranch, shortSHA(res.BaseBefore), res.CommitCount, res.ChangedFilesTotal)
	} else {
		fmt.Fprintf(&b, "merged %s (%s) into %s as %s: %d commit(s), %d file(s)\n",
			res.SessionID, res.Branch, res.BaseBranch, shortSHA(res.MergeCommit), res.CommitCount, res.ChangedFilesTotal)
	}
	for _, f := range res.ChangedFiles {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	if res.ChangedFilesTotal > len(res.ChangedFiles) {
		fmt.Fprintf(&b, "  ... and %d more\n", res.ChangedFilesTotal-len(res.ChangedFiles))
	}
	if res.Verified {
		b.WriteString("verify command passed on the merge result\n")
	}
	if res.UpdatedCheckout != "" {
		fmt.Fprintf(&b, "fast-forwarded the checkout at %s\n", res.UpdatedCheckout)
	}
	for _, o := range res.Overlaps {
		fmt.Fprintf(&b, "warning: %s (%s) also touches: %s\n", o.SessionID, o.Branch, strings.Join(o.Files, ", "))
	}
	if len(res.Notified) > 0 {
		fmt.Fprintf(&b, "told %s that %s moved\n", strings.Join(res.Notified, ", "), res.BaseBranch)
	}
	switch {
	case res.Pushed:
		fmt.Fprintf(&b, "pushed %s (%s) to origin\n", res.PushedRef, shortSHA(res.PushedCommit))
	case res.PushAttempted:
		fmt.Fprintf(&b, "PUSH FAILED: %s was merged locally but not pushed to origin: %s\n", res.BaseBranch, res.PushError)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
