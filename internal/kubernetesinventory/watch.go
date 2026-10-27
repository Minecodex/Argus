package kubernetesinventory

import (
	"context"
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
)

type Event struct {
	Type   string `json:"type"`
	Object Object `json:"object"`
}
type WatchBatch struct {
	SchemaVersion   string    `json:"schema_version"`
	Events          []Event   `json:"events"`
	ResourceVersion string    `json:"resource_version"`
	ObservedAt      time.Time `json:"observed_at"`
	More            bool      `json:"more"`
}

// Watch returns a bounded batch. Callers reauthorize between batches and resume
// from the last included RV. Expiration returns an error requiring a fresh list;
// it never silently changes the query to "now" and misses events.
func (c Client) Watch(parent context.Context, q Query) (WatchBatch, error) {
	result := WatchBatch{SchemaVersion: SchemaVersion, Events: []Event{}, ResourceVersion: q.ResourceVersion, ObservedAt: c.now()}
	if q.ResourceVersion == "" || len(q.ResourceVersion) > 128 || q.Continue != "" || q.Limit < 1 || q.Limit > 200 || q.UID != "" {
		return result, ErrInvalid
	}
	definition, source, namespaceUID, err := c.scope(parent, q)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	seconds := int64(7)
	options := metav1.ListOptions{ResourceVersion: q.ResourceVersion, LabelSelector: q.Labels, AllowWatchBookmarks: true, TimeoutSeconds: &seconds}
	if q.Name != "" {
		options.FieldSelector = fields.OneTermEqualSelector("metadata.name", q.Name).String()
	}
	stream, err := source.Watch(ctx, options)
	if err != nil {
		return result, apiError(err)
	}
	defer stream.Stop()
	budget := c.MaxBytes
	if budget <= 0 || budget > 4<<20 {
		budget = 1 << 20
	}
	used := 0
	var idle *time.Timer
	var idleChannel <-chan time.Time
	defer func() {
		if idle != nil {
			idle.Stop()
		}
	}()
	finish := func() (WatchBatch, error) {
		if parent.Err() != nil {
			return WatchBatch{}, parent.Err()
		}
		if q.Namespace != "" {
			if _, err := c.namespaceUID(parent, q.Namespace, namespaceUID); err != nil {
				return WatchBatch{}, err
			}
		}
		result.ObservedAt = c.now()
		return result, nil
	}
	for {
		select {
		case <-ctx.Done():
			return finish()
		case <-idleChannel:
			return finish()
		case event, open := <-stream.ResultChan():
			if !open {
				return finish()
			}
			if event.Type == watch.Error {
				if status, ok := event.Object.(*metav1.Status); ok && (status.Code == 410 || status.Reason == metav1.StatusReasonExpired) {
					return WatchBatch{}, ErrExpired
				}
				if value, ok := event.Object.(*unstructured.Unstructured); ok {
					code, _, _ := unstructured.NestedInt64(value.Object, "code")
					if code == 410 {
						return WatchBatch{}, ErrExpired
					}
				}
				return WatchBatch{}, ErrUnavailable
			}
			value, ok := event.Object.(*unstructured.Unstructured)
			if !ok || value.GetResourceVersion() == "" {
				return WatchBatch{}, ErrProtocol
			}
			if event.Type == watch.Bookmark {
				result.ResourceVersion = value.GetResourceVersion()
				continue
			}
			if event.Type != watch.Added && event.Type != watch.Modified && event.Type != watch.Deleted {
				return WatchBatch{}, ErrProtocol
			}
			if q.Namespace != "" && value.GetNamespace() != q.Namespace {
				return WatchBatch{}, ErrIdentityChanged
			}
			eventNamespace := namespaceUID
			if q.AllNamespaces && definition.Namespaced && event.Type != watch.Deleted {
				// Verify the live object before assigning a currently observed NS
				// UID. Deleted objects retain unknown NS identity in cluster scope.
				eventNamespace, err = c.namespaceUID(ctx, value.GetNamespace(), "")
				if err != nil {
					return WatchBatch{}, err
				}
				current, e := c.Dynamic.Resource(definition.GVR()).Namespace(value.GetNamespace()).Get(ctx, value.GetName(), metav1.GetOptions{})
				if e != nil {
					return WatchBatch{}, apiError(e)
				}
				if current.GetUID() != value.GetUID() {
					return WatchBatch{}, ErrIdentityChanged
				}
			}
			projected, e := Project(definition, *value, eventNamespace, c.now())
			if e != nil {
				return WatchBatch{}, e
			}
			item := Event{Type: string(event.Type), Object: projected}
			raw, _ := json.Marshal(item)
			if len(raw) > budget {
				return WatchBatch{}, ErrBudget
			}
			if used+len(raw) > budget {
				result.More = true
				return finish()
			}
			used += len(raw)
			result.Events = append(result.Events, item)
			result.ResourceVersion = value.GetResourceVersion()
			if len(result.Events) >= int(q.Limit) {
				result.More = true
				return finish()
			}
			if idle == nil {
				idle = time.NewTimer(150 * time.Millisecond)
				idleChannel = idle.C
			} else {
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(150 * time.Millisecond)
			}
		}
	}
}
