package command

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/githubtemplate"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	issueCmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")

	RootCmd.AddCommand(issueCmd)
	issueCmd.AddCommand(issueStatusCmd)

	issueCmd.AddCommand(issueCreateCmd)
	issueCreateCmd.Flags().StringP("title", "t", "",
		"Supply a title. Will prompt for one otherwise.")
	issueCreateCmd.Flags().StringP("body", "b", "",
		"Supply a body. Will prompt for one otherwise.")
	issueCreateCmd.Flags().BoolP("web", "w", false, "Open the browser to create an issue")
	issueCreateCmd.Flags().StringSliceP("assignee", "a", nil, "Assign people by their `login`")
	issueCreateCmd.Flags().StringSliceP("label", "l", nil, "Add labels by `name`")
	issueCreateCmd.Flags().StringSliceP("project", "p", nil, "Add the issue to projects by `name`")
	issueCreateCmd.Flags().StringP("milestone", "m", "", "Add the issue to a milestone by `name`")

	issueCmd.AddCommand(issueListCmd)
	issueListCmd.Flags().BoolP("web", "w", false, "Open the browser to list the issue(s)")
	issueListCmd.Flags().StringP("assignee", "a", "", "Filter by assignee")
	issueListCmd.Flags().StringSliceP("label", "l", nil, "Filter by labels")
	issueListCmd.Flags().StringP("state", "s", "open", "Filter by state: {open|closed|all}")
	issueListCmd.Flags().IntP("limit", "L", 30, "Maximum number of issues to fetch")
	issueListCmd.Flags().StringP("author", "A", "", "Filter by author")
	issueListCmd.Flags().String("mention", "", "Filter by mention")
	issueListCmd.Flags().StringP("milestone", "m", "", "Filter by milestone `name`")

	issueCmd.AddCommand(issueViewCmd)
	issueViewCmd.Flags().BoolP("web", "w", false, "Open an issue in the browser")

	issueCmd.AddCommand(issueCloseCmd)
	issueCmd.AddCommand(issueReopenCmd)
}

var issueCmd = &cobra.Command{
	Use:   "issue <command>",
	Short: "Create and view issues",
	Long:  `Work with GitHub issues`,
	Example: heredoc.Doc(`
	$ gh issue list
	$ gh issue create --label bug
	$ gh issue view --web
	`),
	Annotations: map[string]string{
		"IsCore": "true",
		"help:arguments": `An issue can be supplied as argument in any of the following formats:
- by number, e.g. "123"; or
- by URL, e.g. "https://github.com/OWNER/REPO/issues/123".`},
}
var issueCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	Args:  cmdutil.NoArgsQuoteReminder,
	RunE:  issueCreate,
	Example: heredoc.Doc(`
	$ gh issue create --title "I found a bug" --body "Nothing works"
	$ gh issue create --label "bug,help wanted"
	$ gh issue create --label bug --label "help wanted"
	$ gh issue create --assignee monalisa,hubot
	$ gh issue create --project "Roadmap"
	`),
}
var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List and filter issues in this repository",
	Example: heredoc.Doc(`
	$ gh issue list -l "help wanted"
	$ gh issue list -A monalisa
	$ gh issue list --web
	`),
	Args: cmdutil.NoArgsQuoteReminder,
	RunE: issueList,
}
var issueStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of relevant issues",
	Args:  cmdutil.NoArgsQuoteReminder,
	RunE:  issueStatus,
}
var issueViewCmd = &cobra.Command{
	Use:   "view {<number> | <url>}",
	Short: "View an issue",
	Args:  cobra.ExactArgs(1),
	Long: `Display the title, body, and other information about an issue.

With '--web', open the issue in a web browser instead.`,
	RunE: issueView,
}
var issueCloseCmd = &cobra.Command{
	Use:   "close {<number> | <url>}",
	Short: "Close issue",
	Args:  cobra.ExactArgs(1),
	RunE:  issueClose,
}
var issueReopenCmd = &cobra.Command{
	Use:   "reopen {<number> | <url>}",
	Short: "Reopen issue",
	Args:  cobra.ExactArgs(1),
	RunE:  issueReopen,
}

func issueList(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	web, err := cmd.Flags().GetBool("web")
	if err != nil {
		return err
	}

	state, err := cmd.Flags().GetString("state")
	if err != nil {
		return err
	}

	labels, err := cmd.Flags().GetStringSlice("label")
	if err != nil {
		return err
	}

	assignee, err := cmd.Flags().GetString("assignee")
	if err != nil {
		return err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}
	if limit <= 0 {
		return fmt.Errorf("invalid limit: %v", limit)
	}

	author, err := cmd.Flags().GetString("author")
	if err != nil {
		return err
	}

	mention, err := cmd.Flags().GetString("mention")
	if err != nil {
		return err
	}

	milestone, err := cmd.Flags().GetString("milestone")
	if err != nil {
		return err
	}

	if web {
		issueListURL := ghrepo.GenerateRepoURL(baseRepo, "issues")
		openURL, err := shared.ListURLWithQuery(issueListURL, shared.FilterOptions{
			Entity:    "issue",
			State:     state,
			Assignee:  assignee,
			Labels:    labels,
			Author:    author,
			Mention:   mention,
			Milestone: milestone,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		return utils.OpenInBrowser(openURL)
	}

	listResult, err := api.IssueList(apiClient, baseRepo, state, labels, assignee, limit, author, mention, milestone)
	if err != nil {
		return err
	}

	hasFilters := false
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "state", "label", "assignee", "author", "mention", "milestone":
			hasFilters = true
		}
	})

	title := shared.ListHeader(ghrepo.FullName(baseRepo), "issue", len(listResult.Issues), listResult.TotalCount, hasFilters)
	if connectedToTerminal(cmd) {
		fmt.Fprintf(colorableErr(cmd), "\n%s\n\n", title)
	}

	out := cmd.OutOrStdout()

	printIssues(out, "", len(listResult.Issues), listResult.Issues)

	return nil
}

func issueStatus(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	currentUser, err := api.CurrentLoginName(apiClient, baseRepo.RepoHost())
	if err != nil {
		return err
	}

	issuePayload, err := api.IssueStatus(apiClient, baseRepo, currentUser)
	if err != nil {
		return err
	}

	out := colorableOut(cmd)

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Relevant issues in %s\n", ghrepo.FullName(baseRepo))
	fmt.Fprintln(out, "")

	shared.PrintHeader(out, "Issues assigned to you")
	if issuePayload.Assigned.TotalCount > 0 {
		printIssues(out, "  ", issuePayload.Assigned.TotalCount, issuePayload.Assigned.Issues)
	} else {
		message := "  There are no issues assigned to you"
		shared.PrintMessage(out, message)
	}
	fmt.Fprintln(out)

	shared.PrintHeader(out, "Issues mentioning you")
	if issuePayload.Mentioned.TotalCount > 0 {
		printIssues(out, "  ", issuePayload.Mentioned.TotalCount, issuePayload.Mentioned.Issues)
	} else {
		shared.PrintMessage(out, "  There are no issues mentioning you")
	}
	fmt.Fprintln(out)

	shared.PrintHeader(out, "Issues opened by you")
	if issuePayload.Authored.TotalCount > 0 {
		printIssues(out, "  ", issuePayload.Authored.TotalCount, issuePayload.Authored.Issues)
	} else {
		shared.PrintMessage(out, "  There are no issues opened by you")
	}
	fmt.Fprintln(out)

	return nil
}

func issueView(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	issue, _, err := issueFromArg(ctx, apiClient, cmd, args[0])
	if err != nil {
		return err
	}
	openURL := issue.URL

	web, err := cmd.Flags().GetBool("web")
	if err != nil {
		return err
	}

	if web {
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", openURL)
		return utils.OpenInBrowser(openURL)
	}
	if connectedToTerminal(cmd) {
		return printHumanIssuePreview(colorableOut(cmd), issue)
	}

	return printRawIssuePreview(cmd.OutOrStdout(), issue)
}

func issueStateTitleWithColor(state string) string {
	colorFunc := shared.ColorFuncForState(state)
	return colorFunc(strings.Title(strings.ToLower(state)))
}

func printRawIssuePreview(out io.Writer, issue *api.Issue) error {
	assignees := issueAssigneeList(*issue)
	labels := issueLabelList(*issue)
	projects := issueProjectList(*issue)

	// Print empty strings for empty values so the number of metadata lines is consistent when
	// processing many issues with head and grep.
	fmt.Fprintf(out, "title:\t%s\n", issue.Title)
	fmt.Fprintf(out, "state:\t%s\n", issue.State)
	fmt.Fprintf(out, "author:\t%s\n", issue.Author.Login)
	fmt.Fprintf(out, "labels:\t%s\n", labels)
	fmt.Fprintf(out, "comments:\t%d\n", issue.Comments.TotalCount)
	fmt.Fprintf(out, "assignees:\t%s\n", assignees)
	fmt.Fprintf(out, "projects:\t%s\n", projects)
	fmt.Fprintf(out, "milestone:\t%s\n", issue.Milestone.Title)

	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, issue.Body)
	return nil
}

func printHumanIssuePreview(out io.Writer, issue *api.Issue) error {
	now := time.Now()
	ago := now.Sub(issue.CreatedAt)

	// Header (Title and State)
	fmt.Fprintln(out, utils.Bold(issue.Title))
	fmt.Fprint(out, issueStateTitleWithColor(issue.State))
	fmt.Fprintln(out, utils.Gray(fmt.Sprintf(
		" • %s opened %s • %s",
		issue.Author.Login,
		utils.FuzzyAgo(ago),
		utils.Pluralize(issue.Comments.TotalCount, "comment"),
	)))

	// Metadata
	fmt.Fprintln(out)
	if assignees := issueAssigneeList(*issue); assignees != "" {
		fmt.Fprint(out, utils.Bold("Assignees: "))
		fmt.Fprintln(out, assignees)
	}
	if labels := issueLabelList(*issue); labels != "" {
		fmt.Fprint(out, utils.Bold("Labels: "))
		fmt.Fprintln(out, labels)
	}
	if projects := issueProjectList(*issue); projects != "" {
		fmt.Fprint(out, utils.Bold("Projects: "))
		fmt.Fprintln(out, projects)
	}
	if issue.Milestone.Title != "" {
		fmt.Fprint(out, utils.Bold("Milestone: "))
		fmt.Fprintln(out, issue.Milestone.Title)
	}

	// Body
	if issue.Body != "" {
		fmt.Fprintln(out)
		md, err := utils.RenderMarkdown(issue.Body)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, md)
	}
	fmt.Fprintln(out)

	// Footer
	fmt.Fprintf(out, utils.Gray("View this issue on GitHub: %s\n"), issue.URL)
	return nil
}

func issueCreate(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	// NB no auto forking like over in pr create
	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return err
	}

	baseOverride, err := cmd.Flags().GetString("repo")
	if err != nil {
		return err
	}

	var nonLegacyTemplateFiles []string
	if baseOverride == "" {
		if rootDir, err := git.ToplevelDir(); err == nil {
			// TODO: figure out how to stub this in tests
			nonLegacyTemplateFiles = githubtemplate.FindNonLegacy(rootDir, "ISSUE_TEMPLATE")
		}
	}

	title, err := cmd.Flags().GetString("title")
	if err != nil {
		return fmt.Errorf("could not parse title: %w", err)
	}
	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return fmt.Errorf("could not parse body: %w", err)
	}

	assignees, err := cmd.Flags().GetStringSlice("assignee")
	if err != nil {
		return fmt.Errorf("could not parse assignees: %w", err)
	}
	labelNames, err := cmd.Flags().GetStringSlice("label")
	if err != nil {
		return fmt.Errorf("could not parse labels: %w", err)
	}
	projectNames, err := cmd.Flags().GetStringSlice("project")
	if err != nil {
		return fmt.Errorf("could not parse projects: %w", err)
	}
	var milestoneTitles []string
	if milestoneTitle, err := cmd.Flags().GetString("milestone"); err != nil {
		return fmt.Errorf("could not parse milestone: %w", err)
	} else if milestoneTitle != "" {
		milestoneTitles = append(milestoneTitles, milestoneTitle)
	}

	if isWeb, err := cmd.Flags().GetBool("web"); err == nil && isWeb {
		openURL := ghrepo.GenerateRepoURL(baseRepo, "issues/new")
		if title != "" || body != "" {
			openURL, err = shared.WithPrAndIssueQueryParams(openURL, title, body, assignees, labelNames, projectNames, milestoneTitles)
			if err != nil {
				return err
			}
		} else if len(nonLegacyTemplateFiles) > 1 {
			openURL += "/choose"
		}
		if connectedToTerminal(cmd) {
			cmd.Printf("Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	}

	fmt.Fprintf(colorableErr(cmd), "\nCreating issue in %s\n\n", ghrepo.FullName(baseRepo))

	repo, err := api.GitHubRepo(apiClient, baseRepo)
	if err != nil {
		return err
	}
	if !repo.HasIssuesEnabled {
		return fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(baseRepo))
	}

	action := shared.SubmitAction
	tb := shared.IssueMetadataState{
		Type:       shared.IssueMetadata,
		Assignees:  assignees,
		Labels:     labelNames,
		Projects:   projectNames,
		Milestones: milestoneTitles,
	}

	interactive := !(cmd.Flags().Changed("title") && cmd.Flags().Changed("body"))

	if interactive && !connectedToTerminal(cmd) {
		return fmt.Errorf("must provide --title and --body when not attached to a terminal")
	}

	if interactive {
		var legacyTemplateFile *string
		if baseOverride == "" {
			if rootDir, err := git.ToplevelDir(); err == nil {
				// TODO: figure out how to stub this in tests
				legacyTemplateFile = githubtemplate.FindLegacy(rootDir, "ISSUE_TEMPLATE")
			}
		}

		editorCommand, err := cmdutil.DetermineEditor(ctx.Config)
		if err != nil {
			return err
		}

		err = shared.TitleBodySurvey(defaultStreams, editorCommand, &tb, apiClient, baseRepo, title, body, shared.Defaults{}, nonLegacyTemplateFiles, legacyTemplateFile, false, repo.ViewerCanTriage())
		if err != nil {
			return fmt.Errorf("could not collect title and/or body: %w", err)
		}

		action = tb.Action

		if tb.Action == shared.CancelAction {
			fmt.Fprintln(cmd.ErrOrStderr(), "Discarding.")

			return nil
		}

		if title == "" {
			title = tb.Title
		}
		if body == "" {
			body = tb.Body
		}
	} else {
		if title == "" {
			return fmt.Errorf("title can't be blank")
		}
	}

	if action == shared.PreviewAction {
		openURL := ghrepo.GenerateRepoURL(baseRepo, "issues/new")
		openURL, err = shared.WithPrAndIssueQueryParams(openURL, title, body, assignees, labelNames, projectNames, milestoneTitles)
		if err != nil {
			return err
		}
		// TODO could exceed max url length for explorer
		fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		return utils.OpenInBrowser(openURL)
	} else if action == shared.SubmitAction {
		params := map[string]interface{}{
			"title": title,
			"body":  body,
		}

		err = shared.AddMetadataToIssueParams(apiClient, baseRepo, params, &tb)
		if err != nil {
			return err
		}

		newIssue, err := api.IssueCreate(apiClient, repo, params)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), newIssue.URL)
	} else {
		panic("Unreachable state")
	}

	return nil
}

func printIssues(w io.Writer, prefix string, totalCount int, issues []api.Issue) {
	io := &iostreams.IOStreams{Out: w}
	io.SetStdoutTTY(utils.IsTerminal(w))
	table := utils.NewTablePrinter(io)
	for _, issue := range issues {
		issueNum := strconv.Itoa(issue.Number)
		if table.IsTTY() {
			issueNum = "#" + issueNum
		}
		issueNum = prefix + issueNum
		labels := issueLabelList(issue)
		if labels != "" && table.IsTTY() {
			labels = fmt.Sprintf("(%s)", labels)
		}
		now := time.Now()
		ago := now.Sub(issue.UpdatedAt)
		table.AddField(issueNum, nil, shared.ColorFuncForState(issue.State))
		if !table.IsTTY() {
			table.AddField(issue.State, nil, nil)
		}
		table.AddField(text.ReplaceExcessiveWhitespace(issue.Title), nil, nil)
		table.AddField(labels, nil, utils.Gray)
		if table.IsTTY() {
			table.AddField(utils.FuzzyAgo(ago), nil, utils.Gray)
		} else {
			table.AddField(issue.UpdatedAt.String(), nil, nil)
		}
		table.EndRow()
	}
	_ = table.Render()
	remaining := totalCount - len(issues)
	if remaining > 0 {
		fmt.Fprintf(w, utils.Gray("%sAnd %d more\n"), prefix, remaining)
	}
}

func issueAssigneeList(issue api.Issue) string {
	if len(issue.Assignees.Nodes) == 0 {
		return ""
	}

	AssigneeNames := make([]string, 0, len(issue.Assignees.Nodes))
	for _, assignee := range issue.Assignees.Nodes {
		AssigneeNames = append(AssigneeNames, assignee.Login)
	}

	list := strings.Join(AssigneeNames, ", ")
	if issue.Assignees.TotalCount > len(issue.Assignees.Nodes) {
		list += ", …"
	}
	return list
}

func issueLabelList(issue api.Issue) string {
	if len(issue.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, 0, len(issue.Labels.Nodes))
	for _, label := range issue.Labels.Nodes {
		labelNames = append(labelNames, label.Name)
	}

	list := strings.Join(labelNames, ", ")
	if issue.Labels.TotalCount > len(issue.Labels.Nodes) {
		list += ", …"
	}
	return list
}

func issueProjectList(issue api.Issue) string {
	if len(issue.ProjectCards.Nodes) == 0 {
		return ""
	}

	projectNames := make([]string, 0, len(issue.ProjectCards.Nodes))
	for _, project := range issue.ProjectCards.Nodes {
		colName := project.Column.Name
		if colName == "" {
			colName = "Awaiting triage"
		}
		projectNames = append(projectNames, fmt.Sprintf("%s (%s)", project.Project.Name, colName))
	}

	list := strings.Join(projectNames, ", ")
	if issue.ProjectCards.TotalCount > len(issue.ProjectCards.Nodes) {
		list += ", …"
	}
	return list
}

func issueClose(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	issue, baseRepo, err := issueFromArg(ctx, apiClient, cmd, args[0])
	if err != nil {
		return err
	}

	if issue.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Issue #%d (%s) is already closed\n", utils.Yellow("!"), issue.Number, issue.Title)
		return nil
	}

	err = api.IssueClose(apiClient, baseRepo, *issue)
	if err != nil {
		return err
	}

	fmt.Fprintf(colorableErr(cmd), "%s Closed issue #%d (%s)\n", utils.Red("✔"), issue.Number, issue.Title)

	return nil
}

func issueReopen(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	issue, baseRepo, err := issueFromArg(ctx, apiClient, cmd, args[0])
	if err != nil {
		return err
	}

	if !issue.Closed {
		fmt.Fprintf(colorableErr(cmd), "%s Issue #%d (%s) is already open\n", utils.Yellow("!"), issue.Number, issue.Title)
		return nil
	}

	err = api.IssueReopen(apiClient, baseRepo, *issue)
	if err != nil {
		return err
	}

	fmt.Fprintf(colorableErr(cmd), "%s Reopened issue #%d (%s)\n", utils.Green("✔"), issue.Number, issue.Title)

	return nil
}
