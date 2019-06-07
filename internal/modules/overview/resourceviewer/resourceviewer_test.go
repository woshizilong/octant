package resourceviewer

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	configFake "github.com/heptio/developer-dash/internal/config/fake"
	"github.com/heptio/developer-dash/internal/modules/overview/objectvisitor"
	storeFake "github.com/heptio/developer-dash/internal/objectstore/fake"
	queryerFake "github.com/heptio/developer-dash/internal/queryer/fake"
	"github.com/heptio/developer-dash/internal/testutil"
	"github.com/heptio/developer-dash/pkg/objectstoreutil"
	"github.com/heptio/developer-dash/pkg/view/component"
)

type stubbedVisitor struct{ visitErr error }

var _ objectvisitor.Visitor = (*stubbedVisitor)(nil)

func (v stubbedVisitor) Visit(context.Context, objectvisitor.ClusterObject) error {
	return v.visitErr
}

func stubVisitor(fail bool) ViewerOpt {
	return func(rv *ResourceViewer) error {
		sv := &stubbedVisitor{}
		if fail {
			sv.visitErr = errors.Errorf("fail")
		}

		rv.visitor = sv
		return nil
	}
}

func Test_ResourceViewer(t *testing.T) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment",
			UID:  types.UID("deployment"),
		},
	}

	controller := gomock.NewController(t)
	defer controller.Finish()

	objectStore := storeFake.NewMockObjectStore(controller)

	dashConfig := configFake.NewMockDash(controller)
	dashConfig.EXPECT().ObjectStore().Return(objectStore).AnyTimes()

	rv, err := New(dashConfig, stubVisitor(false))
	require.NoError(t, err)

	ctx := context.Background()

	vc, err := rv.Visit(ctx, deployment)
	require.NoError(t, err)
	assert.NotNil(t, vc)
}

func Test_ResourceViewer_visitor_fails(t *testing.T) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment",
			UID:  types.UID("deployment"),
		},
	}

	controller := gomock.NewController(t)
	defer controller.Finish()

	objectStore := storeFake.NewMockObjectStore(controller)

	dashConfig := configFake.NewMockDash(controller)
	dashConfig.EXPECT().ObjectStore().Return(objectStore).AnyTimes()

	rv, err := New(dashConfig, stubVisitor(true))
	require.NoError(t, err)

	ctx := context.Background()

	_, err = rv.Visit(ctx, deployment)
	require.Error(t, err)
}

func Test_ComponentCache_Get(t *testing.T) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment",
			UID:  types.UID("deployment"),
		},
	}
	ctx := context.TODO()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dashConfig := configFake.NewMockDash(ctrl)

	dashConfig.EXPECT().ObjectPath(gomock.Any(), "apps/v1", "Deployment", "deployment").Return("/path", nil)

	q := queryerFake.NewMockQueryer(ctrl)
	q.EXPECT().Children(gomock.Any(), testutil.ToUnstructured(t, deployment))

	o := storeFake.NewMockObjectStore(ctrl)
	dashConfig.EXPECT().ObjectStore().Return(o).AnyTimes()

	c, err := NewComponentCache(dashConfig)
	if err != nil {
		require.NoError(t, err)
	}
	c.SetQueryer(q)

	rvComponent, err := c.Get(ctx, deployment)
	require.NoError(t, err)

	metadata := rvComponent.GetMetadata()
	text := metadata.Title[0].(*component.Text)

	assert.Equal(t, "resourceViewer", metadata.Type)
	assert.Equal(t, rvComponent.IsEmpty(), false)
	assert.Equal(t, text.Config.Text, "Resource Viewer")
}

func Test_ComponentCache_GetNoQueryer(t *testing.T) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment",
			UID:  types.UID("deployment"),
		},
	}
	ctx := context.TODO()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dashConfig := configFake.NewMockDash(ctrl)

	o := storeFake.NewMockObjectStore(ctrl)
	dashConfig.EXPECT().ObjectStore().Return(o).AnyTimes()

	c, err := NewComponentCache(dashConfig)
	if err != nil {
		require.NoError(t, err)
	}

	_, err = c.Get(ctx, deployment)
	require.Error(t, err, "no queryer set")
}

func Test_ComponentCache_getComponent(t *testing.T) {
	object := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment",
			UID:  types.UID("deployment"),
		},
	}
	ctx := context.TODO()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dashConfig := configFake.NewMockDash(ctrl)

	q := queryerFake.NewMockQueryer(ctrl)

	o := storeFake.NewMockObjectStore(ctrl)
	dashConfig.EXPECT().ObjectStore().Return(o).AnyTimes()

	components, err := lru.New(100)
	require.NoError(t, err)

	c := &componentCache{
		components: components,
		dashConfig: dashConfig,
	}
	c.SetQueryer(q)

	rv, err := c.newResourceViewer(ctx)
	require.NoError(t, err)

	key, err := objectstoreutil.KeyFromObject(object)
	require.NoError(t, err)

	cc, err := c.getComponent(ctx, key, object, rv)
	require.NoError(t, err)

	rvCC := cc.(*component.ResourceViewer)
	assert.Equal(t, "deployment", rvCC.Config.Nodes["emptyID"].Name)
}

func Test_ComponentCache_visit(t *testing.T) {
	object := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment",
			UID:  types.UID("deployment"),
		},
	}
	ctx := context.TODO()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dashConfig := configFake.NewMockDash(ctrl)

	dashConfig.EXPECT().ObjectPath(gomock.Any(), "apps/v1", "Deployment", "deployment").Return("/path", nil)

	q := queryerFake.NewMockQueryer(ctrl)

	o := storeFake.NewMockObjectStore(ctrl)
	dashConfig.EXPECT().ObjectStore().Return(o).AnyTimes()

	components, err := lru.New(100)
	require.NoError(t, err)

	c := &componentCache{
		components: components,
		dashConfig: dashConfig,
	}
	c.SetQueryer(q)

	rv, err := c.newResourceViewer(ctx)
	require.NoError(t, err)

	key, err := objectstoreutil.KeyFromObject(object)
	require.NoError(t, err)

	cc, err := c.getComponent(ctx, key, object, rv)
	require.NoError(t, err)

	rvCC := cc.(*component.ResourceViewer)
	node, ok := rvCC.Config.Nodes["emptyID"]
	assert.Equal(t, ok, true)
	assert.Equal(t, "deployment", node.Name)

	q.EXPECT().Children(gomock.Any(), testutil.ToUnstructured(t, object))

	done, _ := c.visit(ctx, key, object, rv)
	newKey := <-done

	cc, err = c.getComponent(ctx, newKey, object, rv)
	require.NoError(t, err)

	rvCC = cc.(*component.ResourceViewer)
	_, ok = rvCC.Config.Nodes["emptyID"]
	assert.Equal(t, ok, false)

	node, ok = rvCC.Config.Nodes["deployment"]
	assert.Equal(t, "deployment", node.Name)
}