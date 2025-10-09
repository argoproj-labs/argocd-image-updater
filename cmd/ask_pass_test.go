package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/argoproj/argo-cd/v3/util/askpass"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func init() {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()
	askpass.RegisterAskPassServiceServer(s, &mockAskPassServer{})
	go func() {
		_ = s.Serve(lis)
	}()
}

type mockAskPassServer struct {
	askpass.UnimplementedAskPassServiceServer
}

func (m *mockAskPassServer) GetCredentials(ctx context.Context, req *askpass.CredentialsRequest) (*askpass.CredentialsResponse, error) {
	return &askpass.CredentialsResponse{Username: "testuser", Password: "testpassword"}, nil
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func NewTestCommand() *cobra.Command {
	cmd := NewAskPassCommand()
	cmd.Run = func(c *cobra.Command, args []string) {
		ctx := c.Context()
		if len(args) != 1 {
			fmt.Fprintf(c.ErrOrStderr(), "expected 1 argument, got %d\n", len(args))
			return
		}
		nonce := os.Getenv(askpass.ASKPASS_NONCE_ENV)
		if nonce == "" {
			fmt.Fprintf(c.ErrOrStderr(), "%s is not set\n", askpass.ASKPASS_NONCE_ENV)
			return
		}
		// nolint:staticcheck
		conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(bufDialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			fmt.Fprintf(c.ErrOrStderr(), "failed to connect: %v\n", err)
			return
		}
		defer conn.Close()
		client := askpass.NewAskPassServiceClient(conn)
		creds, err := client.GetCredentials(ctx, &askpass.CredentialsRequest{Nonce: nonce})
		if err != nil {
			fmt.Fprintf(c.ErrOrStderr(), "failed to get credentials: %v\n", err)
			return
		}
		switch {
		case strings.HasPrefix(args[0], "Username"):
			fmt.Fprintln(c.OutOrStdout(), creds.Username)
		case strings.HasPrefix(args[0], "Password"):
			fmt.Fprintln(c.OutOrStdout(), creds.Password)
		default:
			fmt.Fprintf(c.ErrOrStderr(), "unknown credential type '%s'\n", args[0])
		}
	}
	return cmd
}

func TestNewAskPassCommand(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		envNonce    string
		expectedOut string
		expectedErr string
	}{
		{"no arguments", []string{}, "testnonce", "", "expected 1 argument, got 0"},
		{"missing nonce", []string{"Username"}, "", "", fmt.Sprintf("%s is not set", askpass.ASKPASS_NONCE_ENV)},
		{"valid username request", []string{"Username"}, "testnonce", "testuser", ""},
		{"valid password request", []string{"Password"}, "testnonce", "testpassword", ""},
		{"unknown credential type", []string{"Unknown"}, "testnonce", "", "unknown credential type 'Unknown'"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Clearenv()
			if tc.envNonce != "" {
				os.Setenv(askpass.ASKPASS_NONCE_ENV, tc.envNonce)
			}

			var stdout, stderr bytes.Buffer
			command := NewTestCommand()
			command.SetArgs(tc.args)
			command.SetOut(&stdout)
			command.SetErr(&stderr)

			err := command.Execute()

			if tc.expectedOut != "" {
				assert.Equal(t, tc.expectedOut, strings.TrimSpace(stdout.String()))
			}

			if tc.expectedErr != "" {
				assert.Contains(t, stderr.String(), tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
