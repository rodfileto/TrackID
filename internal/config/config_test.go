package config

import (
	"strings"
	"testing"
)

func productionConfig() Config {
	return Config{
		Environment: EnvironmentProduction,
		DatabaseURL: "postgres://x", RedisURL: "redis://x",
		S3Endpoint: "http://s3", S3AccessKey: "a", S3SecretKey: "s", S3Bucket: "b",
		JWTSecret: strings.Repeat("k", MinJWTSecretBytes), AppURL: "https://trackid.example",
	}
}

func TestCheckProductionAcceptsAComplete(t *testing.T) {
	if err := productionConfig().CheckProduction(ProcessAPI); err != nil {
		t.Fatal(err)
	}
}

func TestCheckProductionNamesEveryProblem(t *testing.T) {
	c := productionConfig()
	c.S3Bucket, c.JWTSecret, c.AppURL = "", "short", "http://trackid.example"
	err := c.CheckProduction(ProcessAPI)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"S3_BUCKET", "JWT_SECRET", "APP_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error doesn't name %s: %v", want, err)
		}
	}
}

func TestCheckProductionAsksTheWorkerOnlyWhatItUses(t *testing.T) {
	c := productionConfig()
	c.JWTSecret, c.AppURL = "", ""
	if err := c.CheckProduction(ProcessWorker); err != nil {
		t.Fatalf("the worker signs no tokens and sends no links: %v", err)
	}
}

func TestCheckProductionIsSilentInDevelopment(t *testing.T) {
	if err := (Config{Environment: EnvironmentDevelopment}).CheckProduction(ProcessAPI); err != nil {
		t.Fatal(err)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got, err := parseTrustedProxies(" 10.0.0.0/8, 172.18.0.5 ,")
	if err != nil || len(got) != 2 || got[0] != "10.0.0.0/8" || got[1] != "172.18.0.5" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := parseTrustedProxies("nginx"); err == nil {
		t.Fatal("a host name was accepted")
	}
}
