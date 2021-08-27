package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/ryanuber/columnize"

	"github.com/google/go-github/v35/github"
	"golang.org/x/oauth2"
)

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		expectNoError(errors.New("missing required env var 'GITHUB_TOKEN'"))
	}

	var (
		ctx    = context.Background()
		ts     = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc     = oauth2.NewClient(ctx, ts)
		client = github.NewClient(tc)

		bold   = color.New(color.Bold, color.FgWhite)
		italic = color.New(color.Italic)

		from, _ = time.Parse("2006-01-02", "2021-05-01")
		to, _   = time.Parse("2006-01-02", "2021-07-31")

		rs = rfcs(ctx, client)
		ps = prs(ctx, client)
	)

	rs = filterNonVMware(ctx, client, rs)
	rs = filterFromTime(rs, from, to)

	ps = filterNonVMware(ctx, client, ps)
	ps = filterFromTime(ps, from, to)

	rsrt := responseTimes(ctx, client, rs)
	psrt := responseTimes(ctx, client, ps)
	medianResponseTime := median(rsrt, psrt)

	rsRevCount := prReviewsCount(ctx, client, rs)
	psRevCount := prReviewsCount(ctx, client, ps)

	iss := issues(ctx, client, from)
	iss = filterFromTimeIssues(iss, from, to)
	isrt := responseTimesIssues(ctx, client, filterNonVMwareIssues(ctx, client, iss))
	medianResponseTimeIssues := median(isrt)

	bold.Println("Community Health")
	output := []string{
		fmt.Sprintf("RFCs from Non-VMware people | %d", len(rs)),
		fmt.Sprintf("PRs from Non-VMware people | %d", len(ps))}
	fmt.Println(columnize.SimpleFormat(output) + "\n")

	bold.Println("Team Efficiency")
	output = []string{
		fmt.Sprintf("Count of RFC + PR Reviews by team members | %d", rsRevCount+psRevCount),
		fmt.Sprintf("Median response time to RFC + PR | %s", medianResponseTime),
		fmt.Sprintf("Median response time to Issues | %s", medianResponseTimeIssues),
		fmt.Sprintf("Percentage of \"good-first-issues\" | %f%%", goodFirstIssues(iss))}
	fmt.Println(columnize.SimpleFormat(output) + "\n")

	output = []string{
		italic.Sprintf("from: | %s", from.Format(time.Stamp)),
		italic.Sprintf("to: | %s", to.Format(time.Stamp))}
	fmt.Println(columnize.SimpleFormat(output))
}

var isVMwareMapping = map[string]bool{}
var ours = []string{
	"pivotal",
	"pivotal-legacy",
	"vmware",
	"vmware-tanzu",
}

func isVMware(orgs []*github.Organization) bool {
	for _, org := range orgs {
		for _, item := range ours {
			if *org.Login == item {
				return true
			}
		}
	}

	return false
}

func filterNonVMware(ctx context.Context, client *github.Client, prs []*github.PullRequest) []*github.PullRequest {
	var result []*github.PullRequest

	for _, pr := range prs {
		flag, ok := isVMwareMapping[*pr.User.Login]
		if ok && !flag {
			result = append(result, pr)
			continue
		}

		orgs, _, err := client.Organizations.List(ctx, *pr.User.Login, nil)
		expectNoError(err)

		flag = isVMware(orgs)
		if !flag {
			result = append(result, pr)
		}

		isVMwareMapping[*pr.User.Login] = flag
	}

	return result
}

func filterNonVMwareIssues(ctx context.Context, client *github.Client, issues []*github.Issue) []*github.Issue {
	var result []*github.Issue

	for _, issue := range issues {
		flag, ok := isVMwareMapping[*issue.User.Login]
		if ok && !flag {
			result = append(result, issue)
			continue
		}

		orgs, _, err := client.Organizations.List(ctx, *issue.User.Login, nil)
		expectNoError(err)

		flag = isVMware(orgs)
		if !flag {
			result = append(result, issue)
		}

		isVMwareMapping[*issue.User.Login] = flag
	}

	return result
}

func filterFromTime(prs []*github.PullRequest, from time.Time, to time.Time) []*github.PullRequest {
	var result []*github.PullRequest

	for _, pr := range prs {
		if from.Before(pr.GetCreatedAt()) && to.After(pr.GetCreatedAt()) {
			result = append(result, pr)
		}
	}

	return result
}

func filterFromTimeIssues(issues []*github.Issue, from time.Time, to time.Time) []*github.Issue {
	var result []*github.Issue

	for _, issue := range issues {
		if from.Before(issue.GetCreatedAt()) && to.After(issue.GetCreatedAt()) {
			result = append(result, issue)
		}
	}

	return result
}

func rfcs(ctx context.Context, client *github.Client) []*github.PullRequest {
	prs, err := allPRs(ctx, client, "rfcs")
	expectNoError(err)

	return prs
}

func prs(ctx context.Context, client *github.Client) []*github.PullRequest {
	prs1, err := allPRs(ctx, client, "pack")
	expectNoError(err)

	prs2, err := allPRs(ctx, client, "lifecycle")
	expectNoError(err)

	prs3, err := allPRs(ctx, client, "spec")
	expectNoError(err)

	prs4, err := allPRs(ctx, client, "imgutil")
	expectNoError(err)

	prs5, err := allPRs(ctx, client, "docs")
	expectNoError(err)

	var result []*github.PullRequest
	result = append(result, prs1...)
	result = append(result, prs2...)
	result = append(result, prs3...)
	result = append(result, prs4...)
	result = append(result, prs5...)
	return result
}

func goodFirstIssues(issues []*github.Issue) float64 {
	var goodFirstIssueCount int

	for _, issue := range issues {
		if hasGoodFirstIssueLabel(issue) {
			goodFirstIssueCount++
		}
	}

	return (float64(goodFirstIssueCount) / float64(len(issues))) * 100
}

func hasGoodFirstIssueLabel(issue *github.Issue) bool {
	for _, label := range issue.Labels {
		if strings.Contains(label.GetName(), "good-first-issue") || strings.Contains(label.GetName(), "good first issue") {
			return true
		}
	}

	return false
}

func issues(ctx context.Context, client *github.Client, since time.Time) []*github.Issue {
	issues1, err := allIssues(ctx, client, "pack", since)
	expectNoError(err)

	issues2, err := allIssues(ctx, client, "lifecycle", since)
	expectNoError(err)

	issues3, err := allIssues(ctx, client, "spec", since)
	expectNoError(err)

	issues4, err := allIssues(ctx, client, "imgutil", since)
	expectNoError(err)

	issues5, err := allIssues(ctx, client, "docs", since)
	expectNoError(err)

	var result []*github.Issue
	result = append(result, issues1...)
	result = append(result, issues2...)
	result = append(result, issues3...)
	result = append(result, issues4...)
	result = append(result, issues5...)
	return result
}

func prReviewsCount(ctx context.Context, client *github.Client, prs []*github.PullRequest) int {
	var count int

	for _, pr := range prs {
		owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
		repo := pr.GetBase().GetRepo().GetName()
		number := pr.GetNumber()

		reviews, _, err := client.PullRequests.ListReviews(ctx, owner, repo, number, nil)
		expectNoError(err)

		for _, review := range reviews {
			if review.GetState() != "PENDING" {
				flag, ok := isVMwareMapping[review.GetUser().GetLogin()]
				if ok && flag {
					count = count + 1
					continue
				}

				orgs, _, err := client.Organizations.List(ctx, review.GetUser().GetLogin(), nil)
				expectNoError(err)

				flag = isVMware(orgs)
				if flag {
					count = count + 1
				}

				isVMwareMapping[review.GetUser().GetLogin()] = flag
			}
		}
	}

	return count
}

func responseTimes(ctx context.Context, client *github.Client, prs []*github.PullRequest) []time.Duration {
	var ts []time.Duration

	for _, pr := range prs {
		owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
		repo := pr.GetBase().GetRepo().GetName()
		number := pr.GetNumber()

		comments, _, err := client.PullRequests.ListComments(ctx, owner, repo, number, &github.PullRequestListCommentsOptions{
			Sort:      "created",
			Direction: "asc",
			ListOptions: github.ListOptions{
				Page:    1,
				PerPage: 1,
			},
		})
		expectNoError(err)

		if len(comments) == 0 {
			continue
		}

		ts = append(ts, comments[0].GetCreatedAt().Sub(pr.GetCreatedAt()))
	}

	return ts
}

func responseTimesIssues(ctx context.Context, client *github.Client, issues []*github.Issue) []time.Duration {
	var ts []time.Duration

	for _, issue := range issues {
		url := strings.Split(issue.GetRepositoryURL(), "/")
		owner := url[len(url) - 2]
		repo := url[len(url) - 1]
		number := issue.GetNumber()

		comments, _, err := client.Issues.ListComments(ctx, owner, repo, number, &github.IssueListCommentsOptions{
			Sort:      sPtr("created"),
			Direction: sPtr("asc"),
			ListOptions: github.ListOptions{
				Page:    1,
				PerPage: 1,
			},
		})
		expectNoError(err)

		if len(comments) == 0 {
			continue
		}

		ts = append(ts, comments[0].GetCreatedAt().Sub(issue.GetCreatedAt()))
	}

	return ts
}

func allPRs(ctx context.Context, client *github.Client, repo string) ([]*github.PullRequest, error) {
	var (
		result []*github.PullRequest
		page   = 1
	)

	for {
		var err error
		prs, _, err := client.PullRequests.List(ctx, "buildpacks", repo, &github.PullRequestListOptions{
			State: "all", ListOptions: github.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return nil, err
		}

		if len(prs) == 0 {
			return result, nil
		}

		result = append(result, prs...)
		page = page + 1
	}
}

func allIssues(ctx context.Context, client *github.Client, repo string, since time.Time) ([]*github.Issue, error) {
	var (
		result []*github.Issue
		page   = 1
	)

	for {
		var err error
		iss, _, err := client.Issues.ListByRepo(ctx, "buildpacks", repo, &github.IssueListByRepoOptions{
			Since: since, State: "all", ListOptions: github.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return nil, err
		}

		if len(iss) == 0 {
			return result, nil
		}

		result = append(result, iss...)
		page = page + 1
	}
}

func median(ts ...[]time.Duration) time.Duration {
	var elements []time.Duration
	for _, arr := range ts {
		for _, item := range arr {
			elements = append(elements, item)
		}
	}

	sort.Slice(elements, func(i, j int) bool {
		return elements[i] < elements[j]
	})

	mNumber := len(elements) / 2

	if mNumber%2 != 0 {
		return elements[mNumber]
	}

	return (elements[mNumber-1] + elements[mNumber]) / 2
}

func expectNoError(err error) {
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
}

func sPtr(s string) *string {
	return &s
}
