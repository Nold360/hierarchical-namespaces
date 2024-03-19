package apiresources

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

type scope struct{}

func (s *scope) Name() meta.RESTScopeName {
	return meta.RESTScopeNameNamespace
}

func Test_onAdd(t *testing.T) {
	apis := setup()
	testHandler(t, apis, func() { apis.onAdd(nil) })
}

func Test_onUpdate(t *testing.T) {
	apis := setup()
	testHandler(t, apis, func() { apis.onUpdate(nil, nil) })
}

func Test_onDelete(t *testing.T) {
	apis := setup()
	testHandler(t, apis, func() { apis.onDelete(nil) })
}

func setup() *apiResourceWatcher {
	queueRefreshDelay = 1
	client := fakeclientset.NewSimpleClientset()
	fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscovery.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "pods",
					Namespaced: true,
					Kind:       "Pod",
					Version:    "v1",
					Verbs:      []string{"create", "update", "get", "list", "watch", "delete"},
				},
				{
					Name:       "namespaces",
					Namespaced: false,
					Version:    "v1",
					Verbs:      []string{"create", "update", "get", "list", "watch", "delete"},
				},
			},
		},
	}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, &scope{})
	return &apiResourceWatcher{
		client:      fakeDiscovery,
		resourceMap: make(map[string]metav1.APIResource),
	}
}

func testHandler(t *testing.T, apis *apiResourceWatcher, testFn func()) {
	testFn()
	time.Sleep((queueRefreshDelay + 10) * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&apis.toSync))
	wantAPIResources := []metav1.APIResource{
		{
			Name:               "pods",
			Namespaced:         true,
			Kind:               "Pod",
			StorageVersionHash: "xPOwRZ+Yhw8=",
			Version:            "v1",
			Verbs:              []string{"list", "watch"},
		},
	}
	wantResourceMap := map[string]metav1.APIResource{
		"pods": metav1.APIResource{
			Name:               "pods",
			Namespaced:         true,
			Kind:               "Pod",
			StorageVersionHash: "xPOwRZ+Yhw8=",
			Version:            "v1",
			Verbs:              []string{"list", "watch"},
		},
	}
	assert.Equal(t, wantAPIResources, apis.List())
	assert.Equal(t, wantResourceMap, apis.resourceMap)
}

func TestGetKindForResource(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, &scope{})
	apis := apiResourceWatcher{
		mapper: mapper,
	}
	tests := []struct {
		name     string
		gvr      schema.GroupVersionResource
		wantKind string
		wantErr  bool
	}{
		{
			name: "resource exists",
			gvr: schema.GroupVersionResource{
				Version:  "v1",
				Resource: "pods",
			},
			wantKind: "Pod",
		},
		{
			name: "resource does not exist",
			gvr: schema.GroupVersionResource{
				Group:    "badgroup",
				Version:  "badversion",
				Resource: "badresource",
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		got, gotErr := apis.GetKindForResource(test.gvr)
		if test.wantErr {
			assert.Error(t, gotErr)
			continue
		}
		assert.Equal(t, test.wantKind, got)
	}
}

func TestGet(t *testing.T) {
	apis := apiResourceWatcher{
		resourceMap: map[string]metav1.APIResource{
			"pods": metav1.APIResource{
				Name:       "pods",
				Version:    "v1",
				Group:      "",
				Namespaced: true,
			},
			"apps.deployments": metav1.APIResource{
				Name:       "deployments",
				Version:    "v1",
				Group:      "apps",
				Namespaced: true,
			},
		},
	}
	tests := []struct {
		name       string
		resource   string
		group      string
		want       metav1.APIResource
		wantExists bool
	}{
		{
			name:       "invalid resource",
			resource:   "notaresources",
			wantExists: false,
		},
		{
			name:       "invalid group and resource",
			resource:   "notaresources",
			group:      "notagroup",
			wantExists: false,
		},
		{
			name:     "/api/v1/-style resource",
			resource: "pods",
			want: metav1.APIResource{
				Group:      "",
				Version:    "v1",
				Name:       "pods",
				Namespaced: true,
			},
			wantExists: true,
		},
		{
			name:     "/apis/-style resource",
			resource: "deployments",
			group:    "apps",
			want: metav1.APIResource{
				Group:      "apps",
				Version:    "v1",
				Name:       "deployments",
				Namespaced: true,
			},
			wantExists: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, gotExists := apis.Get(test.resource, test.group)
			assert.Equal(t, test.wantExists, gotExists)
			assert.Equal(t, test.want, got)
		})
	}
}
