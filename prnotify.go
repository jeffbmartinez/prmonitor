package main

import (
	"fmt"
	"log/syslog"
	"os"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	logrus_syslog "github.com/Sirupsen/logrus/hooks/syslog"
	gosxnotifier "github.com/deckarep/gosx-notifier"
	"github.com/google/go-github/github"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

const (
	stalePullRequestTimeLimitDays = time.Hour * 2 * 24
)

func init() {
	// Semicolon separated list of repository urls to watch.
	viper.BindEnv("reposToWatch", "PRNOTIFY_REPOS_TO_WATCH")
	viper.BindEnv("githubAPIToken", "PRNOTIFY_GITHUB_API_TOKEN")

	log.SetLevel(log.DebugLevel)

	hook, err := logrus_syslog.NewSyslogHook("", "", syslog.LOG_DEBUG, "")
	if err != nil {
		os.Exit(1)
	}

	log.AddHook(hook)
}

func main() {
	reposToWatch := getReposToWatch()

	ghClient := getGithubClient()

	log.WithFields(log.Fields{
		"repos": reposToWatch,
	}).Info("Starting prnotify, watching repos")

	allOldPullRequestURLs := []string{}
	for _, repo := range reposToWatch {
		repoPieces := strings.Split(repo, "/")
		repoOwner := repoPieces[0]
		repoName := repoPieces[1]
		if len(repoPieces) != 2 || repoOwner == "" || repoName == "" {
			log.WithFields(log.Fields{
				"repo": repo,
			}).Error("Invalid repo name supplied. If repo name is github.com/owner/repo, supplied format is 'owner/repo'")

			continue
		}

		oldPullRequestURLsForRepo := getOldPullRequestsForRepo(ghClient, repoOwner, repoName)
		allOldPullRequestURLs = append(allOldPullRequestURLs, oldPullRequestURLsForRepo...)
	}

	numOldPullRequests := len(allOldPullRequestURLs)

	toastMessage := fmt.Sprintf("There are %d state pull requests", numOldPullRequests)
	toast := gosxnotifier.NewNotification(toastMessage)
	toast.Title = "Stale Pull Requests"
	toast.Subtitle = "Click to see newest stale PR"
	toast.Sound = gosxnotifier.Basso
	toast.Link = allOldPullRequestURLs[0]
	if err := toast.Push(); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"repoURLs": allOldPullRequestURLs,
		}).Error("Problem making notification")
	}

	log.Debugf("%#v", allOldPullRequestURLs)
}

func getGithubClient() *github.Client {
	githubAPIToken := getGithubAPIToken()
	oauth2Token := &oauth2.Token{AccessToken: githubAPIToken}
	tokenSource := oauth2.StaticTokenSource(oauth2Token)
	tokenClient := oauth2.NewClient(oauth2.NoContext, tokenSource)

	return github.NewClient(tokenClient)
}

func getOldPullRequestsForRepo(client *github.Client, repoOwner string, repoName string) []string {
	pullRequestOptions := github.PullRequestListOptions{State: "open"}
	pullRequests, _, err := client.PullRequests.List(repoOwner, repoName, &pullRequestOptions)
	if err != nil {
		log.WithError(err).Error("Couldn't get pull requests")
		return nil
	}

	oldPullRequestURLs := []string{}
	for _, pr := range pullRequests {
		tooManyDaysAgo := time.Now().Add(-1 * stalePullRequestTimeLimitDays)
		if pr.CreatedAt.Before(tooManyDaysAgo) {
			oldPullRequestURLs = append(oldPullRequestURLs, *pr.HTMLURL)
		}
	}

	return oldPullRequestURLs
}

func getReposToWatch() []string {
	repos := viper.GetString("reposToWatch")
	if repos == "" {
		log.Fatal("No repos have been set, nothing to watch")
	}

	return strings.Split(repos, ";")
}

func getGithubAPIToken() string {
	token := viper.GetString("githubAPIToken")
	if token == "" {
		log.Fatal("github api token is not configured")
	}

	return token
}
