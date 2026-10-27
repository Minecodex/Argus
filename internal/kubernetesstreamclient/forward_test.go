package kubernetesstreamclient

import "testing"

func TestForwardClientRejectsRemoteListenersAndUnsafeEndpoints(t *testing.T) {
	for _, raw := range []string{"0.0.0.0:3306", ":3306", "[::]:3306", "localhost:3306", "192.0.2.1:3306"} {
		if l, e := Listen(raw); e == nil {
			l.Close()
			t.Fatal("nonliteral or external listener accepted", raw)
		}
	}
	for _, raw := range []string{"ws://remote.test/v1/kubernetes/sessions/id", "wss://remote.test/v1/kubernetes/sessions/id?ticket=private", "wss://user:password@remote.test/v1/kubernetes/sessions/id", "wss://remote.test/v1/sessions/id", "wss://remote.test/v1/kubernetes/sessions/id#ticket"} {
		if ValidateURL(raw) == nil {
			t.Fatal("unsafe endpoint accepted", raw)
		}
	}
	for _, raw := range []string{"ws://127.0.0.1:12345/v1/kubernetes/sessions/id", "wss://remote.test/v1/kubernetes/sessions/id"} {
		if e := ValidateURL(raw); e != nil {
			t.Fatal(e)
		}
	}
}
