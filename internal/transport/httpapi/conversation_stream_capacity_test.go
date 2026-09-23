package httpapi

import (
	"bufio"
	"context"
	"io"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestConversationSSESlowConsumersStayBoundedAndCancel(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	e, d, u, c := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'SSE capacity',$2,'UTC')", e, "sse-"+e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Stream user')", u, e, d, "sse-"+u.String())
	model := uuid.New()
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'SSE','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", model, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'SSE capacity',$4)", c, e, u, model)
	exec("INSERT INTO conversation_events(id,enterprise_id,conversation_id,sequence,event_type,actor_type,actor_id,payload,content_hash,data_classification) SELECT gen_random_uuid(),$1,$2,n,'assistant_message','model','test',jsonb_build_object('content',repeat('x',32768)),sha256(convert_to(jsonb_build_object('content',repeat('x',32768))::text,'UTF8')),'internal' FROM generate_series(1,600) n", e, c)
	handler := ConversationHandler{Service: conversation.Service{Store: store}}
	principal := identity.Principal{EnterpriseUser: &db.EnterpriseUser{ID: u, EnterpriseID: e, AuthorizationVersion: 1}}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	type consumer struct {
		reader *io.PipeReader
		cancel context.CancelFunc
		done   chan struct{}
	}
	consumers := []consumer{}
	defer func() {
		for _, c := range consumers {
			c.cancel()
			_ = c.reader.Close()
		}
	}()
	for index := 0; index < 8; index++ {
		reader, writer := io.Pipe()
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		consumers = append(consumers, consumer{reader, cancel, done})
		go func() { handler.writeEventStream(ctx, writer, principal, c, 0); close(done) }()
		if _, err := io.ReadFull(reader, make([]byte, 1)); err != nil {
			t.Fatal(err)
		}
		// Leave the remaining frame unread: the producer must not keep loading pages.
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("slow_consumers=8 retained_heap_bytes=%d budget_bytes=%d event_bytes=32768 history_events=600", retained, 128<<20)
	if retained > 128<<20 {
		t.Fatalf("slow consumer retention exceeded capacity budget: %d", retained)
	}
	start := time.Now()
	for _, c := range consumers {
		c.cancel()
	}
	for _, c := range consumers {
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
			t.Fatal("cancelled slow SSE producer remained blocked")
		}
	}
	t.Logf("cancel_drain_ms=%d", time.Since(start).Milliseconds())
	// A consumer reconnects after the last complete event, not its partial frame.
	reader, writer := io.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	defer reader.Close()
	go handler.writeEventStream(ctx, writer, principal, c, 12)
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil || line != "id: 13\n" {
		t.Fatalf("resume cursor skipped/replayed complete events: %q %v", line, err)
	}
}
