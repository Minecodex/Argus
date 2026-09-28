package dashboard

import "os"

func queryTestObjectCredentials() (string, string) {
	access, secret := os.Getenv("ARGUS_QUERY_TEST_OBJECT_ACCESS_KEY"), os.Getenv("ARGUS_QUERY_TEST_OBJECT_SECRET_KEY")
	if access == "" && secret == "" {
		return "planv2-test", "planv2-test-secret"
	}
	return access, secret
}
