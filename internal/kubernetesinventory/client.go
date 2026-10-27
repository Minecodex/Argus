package kubernetesinventory

import (
	"context"
	"encoding/json"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	pathvalidation "k8s.io/apimachinery/pkg/api/validation/path"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
)

type Client struct {
	Dynamic  dynamic.Interface
	MaxBytes int
	Now      func() time.Time
}
type namespaceIdentity struct {
	uid     string
	created time.Time
}

func apiError(err error) error {
	if err == nil {
		return nil
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return ErrForbidden
	}
	if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
		return ErrExpired
	}
	if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
		return ErrIdentityChanged
	}
	return ErrUnavailable
}
func (c Client) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
func (c Client) namespaceUID(ctx context.Context, name, expected string) (string, error) {
	value, err := c.namespaceIdentity(ctx, name, expected)
	return value.uid, err
}
func (c Client) namespaceIdentity(ctx context.Context, name, expected string) (namespaceIdentity, error) {
	value, err := c.Dynamic.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return namespaceIdentity{}, apiError(err)
	}
	id := string(value.GetUID())
	if id == "" || expected != "" && id != expected {
		return namespaceIdentity{}, ErrIdentityChanged
	}
	return namespaceIdentity{uid: id, created: value.GetCreationTimestamp().Time}, nil
}
func (c Client) scope(ctx context.Context, q Query) (Definition, dynamic.ResourceInterface, string, error) {
	d, err := c.Definition(ctx, q.Type)
	if err != nil {
		return d, nil, "", err
	}
	if c.Dynamic == nil {
		return d, nil, "", ErrUnavailable
	}
	if len(q.Continue) > 16384 || len(q.Labels) > 1024 || len(q.Name) > 253 || q.Name != "" && len(pathvalidation.IsValidPathSegmentName(q.Name)) > 0 {
		return d, nil, "", ErrInvalid
	}
	if _, err := labels.Parse(q.Labels); err != nil {
		return d, nil, "", ErrInvalid
	}
	var source dynamic.ResourceInterface = c.Dynamic.Resource(d.GVR())
	nsUID := ""
	if d.Namespaced {
		if q.Namespace == "" {
			if !q.AllNamespaces || q.NamespaceUID != "" {
				return d, nil, "", ErrInvalid
			}
		} else {
			if len(validation.IsDNS1123Label(q.Namespace)) > 0 || q.NamespaceUID == "" {
				return d, nil, "", ErrInvalid
			}
			var err error
			nsUID, err = c.namespaceUID(ctx, q.Namespace, q.NamespaceUID)
			if err != nil {
				return d, nil, "", err
			}
		}
		source = c.Dynamic.Resource(d.GVR()).Namespace(q.Namespace)
	} else if q.Namespace != "" || q.NamespaceUID != "" || q.AllNamespaces {
		return d, nil, "", ErrInvalid
	}
	return d, source, nsUID, nil
}
func (c Client) List(ctx context.Context, q Query) (Page, error) {
	page := Page{SchemaVersion: SchemaVersion, Items: []Object{}, ObservedAt: c.now()}
	if q.Limit < 1 || q.Limit > 200 || q.UID != "" || q.ResourceVersion != "" && q.Continue != "" {
		return page, ErrInvalid
	}
	d, source, nsUID, err := c.scope(ctx, q)
	if err != nil {
		return page, err
	}
	options := metav1.ListOptions{Limit: q.Limit, Continue: q.Continue, LabelSelector: q.Labels, ResourceVersion: q.ResourceVersion}
	if q.Name != "" {
		options.FieldSelector = fields.OneTermEqualSelector("metadata.name", q.Name).String()
	}
	list, err := source.List(ctx, options)
	if err != nil {
		return page, apiError(err)
	}
	budget := c.MaxBytes
	if budget <= 0 || budget > 4<<20 {
		budget = 1 << 20
	}
	used := 0
	namespaces := map[string]namespaceIdentity{}
	for _, value := range list.Items {
		if q.Namespace != "" && value.GetNamespace() != q.Namespace {
			return page, ErrIdentityChanged
		}
		objectNamespaceUID := nsUID
		if d.Namespaced && q.AllNamespaces {
			identity, known := namespaces[value.GetNamespace()]
			if !known {
				identity, err = c.namespaceIdentity(ctx, value.GetNamespace(), "")
				if err != nil {
					return Page{}, err
				}
				namespaces[value.GetNamespace()] = identity
			}
			// Creation times are second-granularity in Kubernetes. For objects
			// at or before the namespace creation time, verify the current UID
			// explicitly so an old list snapshot cannot be assigned to a new NS.
			if identity.created.IsZero() || !value.GetCreationTimestamp().Time.After(identity.created) {
				current, e := c.Dynamic.Resource(d.GVR()).Namespace(value.GetNamespace()).Get(ctx, value.GetName(), metav1.GetOptions{})
				if e != nil {
					return Page{}, apiError(e)
				}
				if current.GetUID() != value.GetUID() {
					return Page{}, ErrIdentityChanged
				}
			}
			objectNamespaceUID = identity.uid
		}
		object, err := Project(d, value, objectNamespaceUID, page.ObservedAt)
		if err != nil {
			return page, err
		}
		raw, _ := json.Marshal(object)
		used += len(raw)
		// Never return a partial server page with its next-page token: that
		// would silently skip objects that did not fit. Retry with a lower limit.
		if used > budget {
			return Page{SchemaVersion: SchemaVersion, Items: []Object{}}, ErrBudget
		}
		page.Items = append(page.Items, object)
	}
	if q.Namespace != "" {
		if _, err = c.namespaceUID(ctx, q.Namespace, nsUID); err != nil {
			return Page{SchemaVersion: SchemaVersion, Items: []Object{}}, err
		}
	}
	for name, identity := range namespaces {
		if _, err = c.namespaceUID(ctx, name, identity.uid); err != nil {
			return Page{}, err
		}
	}
	page.Continue = list.GetContinue()
	page.ResourceVersion = list.GetResourceVersion()
	page.Complete = page.Continue == ""
	return page, nil
}
func (c Client) Get(ctx context.Context, q Query) (Object, error) {
	if q.Name == "" || q.Continue != "" || q.AllNamespaces {
		return Object{}, ErrInvalid
	}
	d, source, nsUID, err := c.scope(ctx, q)
	if err != nil {
		return Object{}, err
	}
	value, err := source.Get(ctx, q.Name, metav1.GetOptions{})
	if err != nil {
		return Object{}, apiError(err)
	}
	if value.GetName() != q.Name || value.GetNamespace() != q.Namespace {
		return Object{}, ErrIdentityChanged
	}
	if q.UID != "" && string(value.GetUID()) != q.UID {
		return Object{}, ErrIdentityChanged
	}
	if q.Namespace != "" {
		if _, err = c.namespaceUID(ctx, q.Namespace, nsUID); err != nil {
			return Object{}, err
		}
	}
	object, err := Project(d, *value, nsUID, c.now())
	if err != nil {
		return Object{}, err
	}
	budget := c.MaxBytes
	if budget <= 0 || budget > 4<<20 {
		budget = 1 << 20
	}
	raw, _ := json.Marshal(object)
	if len(raw) > budget {
		return Object{}, ErrBudget
	}
	return object, nil
}
