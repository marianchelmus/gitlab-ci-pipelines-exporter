package exporter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mvisonneau/gitlab-ci-pipelines-exporter/lib/schemas"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	gitlab "github.com/xanzy/go-gitlab"
	"go.uber.org/ratelimit"
)

// Mocking helpers
func getMockedGitlabClient() (*http.ServeMux, *httptest.Server, *Client) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	opts := []gitlab.ClientOptionFunc{
		gitlab.WithBaseURL(server.URL),
		gitlab.WithoutRetries(),
	}

	gc, _ := gitlab.NewClient("", opts...)

	c := &Client{
		Client:      gc,
		RateLimiter: ratelimit.New(100),
	}

	return mux, server, c
}

// Functions testing
func TestProjectExists(t *testing.T) {
	foo := schemas.Project{
		Name: "foo",
		Parameters: schemas.Parameters{
			RefsRegexpValue: pointy.String("abc"),
		},
	}
	fooClone := foo
	bar := schemas.Project{Name: "bar"}

	config := &schemas.Config{
		Projects: []schemas.Project{foo},
	}

	assert.Equal(t, true, config.ProjectExists(fooClone))
	assert.Equal(t, false, config.ProjectExists(bar))
}

func TestGetProject(t *testing.T) {
	mux, server, c := getMockedGitlabClient()
	defer server.Close()

	project := "foo/bar"
	mux.HandleFunc(fmt.Sprintf("/api/v4/projects/%s", project),
		func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, "GET")
			fmt.Fprint(w, `{"id":1}`)
		})

	p, err := c.getProject(project)
	assert.Nil(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, 1, p.ID)
}

func TestListUserProjects(t *testing.T) {
	mux, server, c := getMockedGitlabClient()
	defer server.Close()

	w := &schemas.Wildcard{
		Search: "bar",
		Owner: struct {
			Name             string `yaml:"name"`
			Kind             string `yaml:"kind"`
			IncludeSubgroups bool   `yaml:"include_subgroups"`
		}{
			Name:             "foo",
			Kind:             "user",
			IncludeSubgroups: false,
		},
		Archived: false,
	}

	mux.HandleFunc(fmt.Sprintf("/api/v4/users/%s/projects", w.Owner.Name),
		func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, "GET")
			fmt.Fprint(w, `[{"id":1},{"id":2}]`)
		})

	projects, err := c.listProjects(w)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(projects))
}

func TestListGroupProjects(t *testing.T) {
	mux, server, c := getMockedGitlabClient()
	defer server.Close()

	w := &schemas.Wildcard{
		Search: "bar",
		Owner: struct {
			Name             string `yaml:"name"`
			Kind             string `yaml:"kind"`
			IncludeSubgroups bool   `yaml:"include_subgroups"`
		}{
			Name:             "foo",
			Kind:             "group",
			IncludeSubgroups: false,
		},
		Archived: false,
	}

	mux.HandleFunc(fmt.Sprintf("/api/v4/groups/%s/projects", w.Owner.Name),
		func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, "GET")
			fmt.Fprint(w, `[{"id":1},{"id":2}]`)
		})

	projects, err := c.listProjects(w)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(projects))
}

func TestListProjects(t *testing.T) {
	mux, server, c := getMockedGitlabClient()
	defer server.Close()

	w := &schemas.Wildcard{
		Search: "bar",
		Owner: struct {
			Name             string `yaml:"name"`
			Kind             string `yaml:"kind"`
			IncludeSubgroups bool   `yaml:"include_subgroups"`
		}{
			Name:             "",
			Kind:             "",
			IncludeSubgroups: false,
		},
		Archived: false,
	}

	mux.HandleFunc("/api/v4/projects",
		func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, "GET")
			fmt.Fprint(w, `[{"id":1},{"id":2}]`)
		})

	projects, err := c.listProjects(w)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(projects))
}

func TestListProjectsAPIError(t *testing.T) {
	mux, server, c := getMockedGitlabClient()
	defer server.Close()

	w := &schemas.Wildcard{
		Search: "bar",
		Owner: struct {
			Name             string `yaml:"name"`
			Kind             string `yaml:"kind"`
			IncludeSubgroups bool   `yaml:"include_subgroups"`
		}{
			Name: "foo",
			Kind: "user",
		},
		Archived: false,
	}

	mux.HandleFunc(fmt.Sprintf("/api/v4/users/%s/projects", w.Owner.Name),
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500 - Something bad happened!"))
		})

	_, err := c.listProjects(w)
	assert.NotNil(t, err)
	assert.Equal(t, true, strings.HasPrefix(err.Error(), "unable to list projects with search pattern"))
}

// TODO: Reimplement these tests
// Here an example of concurrent execution of projects polling
// func TestProjectPolling(t *testing.T) {
// 	projects := []schemas.Project{{Name: "test1"}, {Name: "test2"}, {Name: "test3"}, {Name: "test4"}}
// 	until := make(chan struct{})
// 	defer close(until)
// 	_, _, c := getMockedGitlabClient()
// 	// provided we are able to intercept an error from from pollProject method
// 	// we can iterate over a channel of Project and collect its results
// 	assert.Equal(t, len(projects), pollingResult(until, readProjects(until, projects...), c, t))
// }

// func pollingResult(until <-chan struct{}, projects <-chan schemas.Project, client *Client, t *testing.T) (numErrs int) {
// 	for i := range projects {
// 		select {
// 		case <-until:
// 			return numErrs
// 		default:
// 			if assert.Error(t, client.pollProject(i)) {
// 				numErrs++
// 			}
// 		}
// 	}
// 	return numErrs
// }

// func TestPollProjectsRefs(t *testing.T) {
// 	message := "some error"
// 	doing := func() func(*ProjectRef) error {
// 		return func(*ProjectRef) error {
// 			// set the already polled refs, simulate the pollProject(p Project) set of Client.hasPolledOnInit
// 			// return an error to count them afterwards
// 			return fmt.Errorf(message)
// 		}
// 	}
// 	testProjects := ProjectsRefs{}
// 	testProjects[1] = map[string]*ProjectRef{"master": &ProjectRef{}}
// 	testProjects[2] = map[string]*ProjectRef{"master": &ProjectRef{}}

// 	until := make(chan struct{})
// 	errCh := pollProjectsRefs(2, doing(), until, testProjects)
// 	var errCount int
// 	for err := range errCh {
// 		if assert.Error(t, err) {
// 			assert.Equal(t, err.Error(), message)
// 			errCount++
// 		}
// 	}
// 	assert.Equal(t, len(testProjects), errCount)
// }

func readProjects(until chan struct{}, projects ...schemas.Project) <-chan schemas.Project {
	p := make(chan schemas.Project)
	go func() {
		defer close(p)
		for _, i := range projects {
			select {
			case <-until:
				return
			case p <- i:
			}
		}
	}()
	return p
}
