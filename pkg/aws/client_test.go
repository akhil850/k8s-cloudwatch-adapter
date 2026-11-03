package aws

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awslabs/k8s-cloudwatch-adapter/pkg/apis/metrics/v1alpha1"
)

func isolateAWSConfig(t *testing.T) {
	t.Helper()
	for _, key := range []string{"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_EC2_METADATA_DISABLED", "AWS_EC2_METADATA_V1_DISABLED", "AWS_DEFAULTS_MODE"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
}

func TestRegionPrecedenceWithoutMetadata(t *testing.T) {
	for _, tt := range []struct {
		name, spec, region, defaultRegion, want string
	}{
		{name: "spec overrides environment", spec: "eu-west-1", region: "us-west-2", defaultRegion: "us-east-1", want: "eu-west-1"},
		{name: "AWS_REGION", region: "us-west-2", defaultRegion: "us-east-1", want: "us-west-2"},
		{name: "AWS_DEFAULT_REGION", defaultRegion: "us-east-1", want: "us-east-1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAWSConfig(t)
			t.Setenv("AWS_REGION", tt.region)
			t.Setenv("AWS_DEFAULT_REGION", tt.defaultRegion)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "metadata must not be used", http.StatusForbidden)
			}))
			defer server.Close()
			t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", server.URL)
			manager := NewCloudWatchManager().(*cloudwatchManager)
			manager.localRegion = "ap-south-1"
			client, err := manager.getClient(nil, &tt.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got := client.Options().Region; got != tt.want {
				t.Fatalf("region = %q, want %q", got, tt.want)
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("made %d metadata requests despite configured region", got)
			}
		})
	}
}

func TestMetadataRegionFallback(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{name: "IMDSv2 identity document", body: `{"region":"us-west-2"}`, status: http.StatusOK},
		{name: "unauthorized", body: "unauthorized", status: http.StatusUnauthorized, wantError: true},
		{name: "empty response", status: http.StatusOK, wantError: true},
		{name: "missing region", body: `{}`, status: http.StatusOK, wantError: true},
		{name: "invalid response", body: "not JSON", status: http.StatusOK, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAWSConfig(t)
			var tokenCalls, documentCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/latest/api/token":
					tokenCalls.Add(1)
					if r.Method != http.MethodPut {
						t.Errorf("token method = %s", r.Method)
					}
					w.Header().Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
					fmt.Fprint(w, "test-token")
				case "/latest/dynamic/instance-identity/document":
					documentCalls.Add(1)
					if r.Header.Get("X-aws-ec2-metadata-token") != "test-token" {
						t.Error("metadata token missing")
					}
					w.WriteHeader(tt.status)
					fmt.Fprint(w, tt.body)
				default:
					t.Errorf("unexpected metadata path: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", server.URL)
			manager := &cloudwatchManager{}
			client, err := manager.getClient(nil, nil)
			if tt.wantError {
				if err == nil || client != nil {
					t.Fatalf("invalid metadata accepted: client %v, err %v", client, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if got := client.Options().Region; got != "us-west-2" {
					t.Fatalf("region = %q", got)
				}
				if _, err := manager.getClient(nil, nil); err != nil {
					t.Fatal(err)
				}
				if got := documentCalls.Load(); got != 1 {
					t.Fatalf("successful metadata lookup was not cached: %d requests", got)
				}
			}
			if tokenCalls.Load() == 0 || documentCalls.Load() == 0 {
				t.Fatal("IMDSv2 lookup did not occur")
			}
		})
	}
}

func TestMetadataRegionTimeout(t *testing.T) {
	isolateAWSConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", server.URL)
	start := time.Now()
	if region := GetLocalRegion(); region != "" {
		t.Fatalf("region = %q, want empty on timeout", region)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("metadata timeout took %v", elapsed)
	}
}

func TestMissingRegionReturnsQueryError(t *testing.T) {
	isolateAWSConfig(t)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	results, err := NewCloudWatchManager().QueryCloudWatch(v1alpha1.ExternalMetric{})
	if err == nil || results != nil {
		t.Fatalf("missing region must return an error: got %v, err %v", results, err)
	}
}

func TestMetadataRegionRecoversAfterFailure(t *testing.T) {
	isolateAWSConfig(t)
	var documentCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest/api/token" {
			w.Header().Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
			fmt.Fprint(w, "test-token")
			return
		}
		if documentCalls.Add(1) == 1 {
			fmt.Fprint(w, `{}`)
			return
		}
		fmt.Fprint(w, `{"region":"us-west-2"}`)
	}))
	defer server.Close()
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", server.URL)
	manager := &cloudwatchManager{}
	if _, err := manager.getClient(nil, nil); err == nil {
		t.Fatal("expected first metadata lookup to fail")
	}
	client, err := manager.getClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := client.Options().Region; got != "us-west-2" {
		t.Fatalf("region after recovery = %q", got)
	}
}

func TestSharedConfigRegionAndInvalidConfig(t *testing.T) {
	for _, tt := range []struct {
		name, config string
		wantError    bool
	}{
		{name: "shared config region", config: "[default]\nregion = eu-central-1\n"},
		{name: "invalid shared config", config: "[default]\nmax_attempts = invalid\n", wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAWSConfig(t)
			t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
			if err := os.WriteFile(os.Getenv("AWS_CONFIG_FILE"), []byte(tt.config), 0600); err != nil {
				t.Fatal(err)
			}
			client, err := (&cloudwatchManager{}).getClient(nil, nil)
			if tt.wantError {
				if err == nil || client != nil {
					t.Fatalf("invalid config accepted: client %v, err %v", client, err)
				}
				if !strings.Contains(err.Error(), "unable to load AWS SDK config") {
					t.Fatalf("unexpected config error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := client.Options().Region; got != "eu-central-1" {
				t.Fatalf("region = %q", got)
			}
		})
	}
}
