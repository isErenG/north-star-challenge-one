package config

import "testing"

func TestSecretMapboxTokenIsNeverExposed(t *testing.T) {
	if publicMapboxToken("sk.secret") != "" || publicMapboxToken("") != "" {
		t.Fatal("secret token exposed")
	}
	if publicMapboxToken(" pk.public ") != "pk.public" {
		t.Fatal("public token dropped")
	}
}
