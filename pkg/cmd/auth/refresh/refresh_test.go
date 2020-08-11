package refresh

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func Test_NewCmdRefresh(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants RefreshOptions
	}{
		{
			name: "no arguments",
			wants: RefreshOptions{
				Hostname: "",
				Scopes:   []string{},
			},
		},
		{
			name: "hostname",
			cli:  "-h aline.cedrac",
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
				Scopes:   []string{},
			},
		},
		{
			name: "one scope",
			cli:  "--scopes repo:invite",
			wants: RefreshOptions{
				Scopes: []string{"repo:invite"},
			},
		},
		{
			name: "scopes",
			cli:  "--scopes repo:invite,read:public_key",
			wants: RefreshOptions{
				Scopes: []string{"repo:invite", "read:public_key"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *RefreshOptions
			cmd := NewCmdRefresh(f, func(opts *RefreshOptions) error {
				gotOpts = opts
				return nil
			})
			// TODO cobra hack-around
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)
			assert.Equal(t, tt.wants.Hostname, gotOpts.Hostname)
			assert.Equal(t, tt.wants.Scopes, gotOpts.Scopes)
		})

	}
}

func Test_refreshRun(t *testing.T) {
	tests := []struct {
		name     string
		opts     *RefreshOptions
		askStubs func(*prompt.AskStubber)
		cfgHosts []string
		wantErr  *regexp.Regexp
		ghtoken  string
		nontty   bool
	}{
		{
			name:    "GITHUB_TOKEN set",
			opts:    &RefreshOptions{},
			ghtoken: "abc123",
			wantErr: regexp.MustCompile(`GITHUB_TOKEN is present in your environment`),
		},
		{
			name:    "non tty",
			opts:    &RefreshOptions{},
			nontty:  true,
			wantErr: regexp.MustCompile(`not attached to a terminal;`),
		},
		// TODO not logged into any hosts
		// TODO no hostname, one host configured
		// TODO no hostname, multiple hosts configured
		// TODO hostname provided but not configured
		// TODO hostname provided and is configured
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghtoken := os.Getenv("GITHUB_TOKEN")
			defer func() {
				os.Setenv("GITHUB_TOKEN", ghtoken)
			}()
			os.Setenv("GITHUB_TOKEN", tt.ghtoken)
			io, _, _, _ := iostreams.Test()

			io.SetStdinTTY(!tt.nontty)
			io.SetStdoutTTY(!tt.nontty)

			tt.opts.IO = io
			cfg := config.NewBlankConfig()
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}
			for _, hostname := range tt.cfgHosts {
				_ = cfg.Set(hostname, "oauth_token", "abc123")
			}
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.GraphQL(`query UserCurrent\b`),
				httpmock.StringResponse(`{"data":{"viewer":{"login":"cybilb"}}}`))

			// TODO ensure that auth flow is properly stubbed--command stubber?
			//tt.opts.HttpClient = func() (*http.Client, error) {
			//	return &http.Client{Transport: reg}, nil
			//}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

			as, teardown := prompt.InitAskStubber()
			defer teardown()
			if tt.askStubs != nil {
				tt.askStubs(as)
			}

			err := refreshRun(tt.opts)
			assert.Equal(t, tt.wantErr == nil, err == nil)
			if err != nil {
				if tt.wantErr != nil {
					assert.True(t, tt.wantErr.MatchString(err.Error()))
					return
				} else {
					t.Fatalf("unexpected error: %s", err)
				}
			}
		})
	}
}
