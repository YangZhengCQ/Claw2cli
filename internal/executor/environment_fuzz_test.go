package executor

import "testing"

func FuzzIsSafeEnvVar(f *testing.F) {
	f.Add("PATH=/usr/bin")
	f.Add("AWS_SECRET_ACCESS_KEY=secret")
	f.Add("NODE_AUTH_TOKEN=token")
	f.Add("")
	f.Add("=")
	f.Add("VERY_LONG_" + string(make([]byte, 10000)))

	f.Fuzz(func(t *testing.T, env string) {
		isSafeEnvVar(env) // must not panic
	})
}
