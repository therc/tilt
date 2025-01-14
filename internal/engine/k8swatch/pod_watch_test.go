package k8swatch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/windmilleng/tilt/internal/k8s/testyaml"
	"github.com/windmilleng/tilt/internal/testutils"
	"github.com/windmilleng/tilt/internal/testutils/manifestbuilder"
	"github.com/windmilleng/tilt/internal/testutils/podbuilder"
	"github.com/windmilleng/tilt/internal/testutils/tempdir"

	"github.com/windmilleng/tilt/internal/k8s"
	"github.com/windmilleng/tilt/internal/store"
	"github.com/windmilleng/tilt/pkg/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

func TestPodWatch(t *testing.T) {
	f := newPWFixture(t)
	defer f.TearDown()

	manifest := f.addManifestWithSelectors("server")

	f.pw.OnChange(f.ctx, f.store)

	ls := k8s.ManagedByTiltSelector()
	pb := podbuilder.New(t, manifest)
	p := pb.Build()

	// Simulate the Deployment UID in the engine state
	f.addDeployedUID(manifest, pb.DeploymentUID())
	f.kClient.InjectEntityByName(pb.ObjectTreeEntities()...)

	f.kClient.EmitPod(ls, p)

	f.assertWatchedSelectors(ls)

	f.assertObservedPods(p)
}

func TestPodWatchChangeEventBeforeUID(t *testing.T) {
	f := newPWFixture(t)
	defer f.TearDown()

	manifest := f.addManifestWithSelectors("server")

	f.pw.OnChange(f.ctx, f.store)

	ls := k8s.ManagedByTiltSelector()
	pb := podbuilder.New(t, manifest)
	p := pb.Build()

	f.kClient.InjectEntityByName(pb.ObjectTreeEntities()...)
	f.kClient.EmitPod(ls, p)
	f.assertObservedPods()

	// Simulate the Deployment UID in the engine state after
	// the pod event.
	f.addDeployedUID(manifest, pb.DeploymentUID())

	f.assertObservedPods(p)
}

func TestPodWatchExtraSelectors(t *testing.T) {
	f := newPWFixture(t)
	defer f.TearDown()

	ls := k8s.ManagedByTiltSelector()
	ls1 := labels.Set{"foo": "bar"}.AsSelector()
	ls2 := labels.Set{"baz": "quu"}.AsSelector()
	manifest := f.addManifestWithSelectors("server", ls1, ls2)

	f.pw.OnChange(f.ctx, f.store)

	f.assertWatchedSelectors(ls, ls1, ls2)

	p := podbuilder.New(t, manifest).
		WithPodLabel("foo", "bar").
		Build()
	f.kClient.EmitPod(ls1, p)

	f.assertObservedPods(p)
	assert.Equal(t, []model.ManifestName{manifest.Name}, f.manifestNames)
}

func TestPodWatchHandleSelectorChange(t *testing.T) {
	f := newPWFixture(t)
	defer f.TearDown()

	ls := k8s.ManagedByTiltSelector()
	ls1 := labels.Set{"foo": "bar"}.AsSelector()
	manifest := f.addManifestWithSelectors("server1", ls1)

	f.pw.OnChange(f.ctx, f.store)

	f.assertWatchedSelectors(ls, ls1)

	p := podbuilder.New(t, manifest).
		WithPodLabel("foo", "bar").
		Build()
	f.kClient.EmitPod(ls1, p)

	f.assertObservedPods(p)
	f.clearPods()

	ls2 := labels.Set{"baz": "quu"}.AsSelector()
	manifest2 := f.addManifestWithSelectors("server2", ls2)
	f.removeManifest("server1")

	f.pw.OnChange(f.ctx, f.store)

	f.assertWatchedSelectors(ls, ls2)

	pb2 := podbuilder.New(t, manifest2).WithPodID("pod2")
	p2 := pb2.Build()
	f.addDeployedUID(manifest2, pb2.DeploymentUID())
	f.kClient.InjectEntityByName(pb2.ObjectTreeEntities()...)
	f.kClient.EmitPod(ls, p2)
	f.assertObservedPods(p2)
	f.clearPods()

	p3 := podbuilder.New(t, manifest2).
		WithPodID("pod3").
		WithPodLabel("foo", "bar").
		Build()
	f.kClient.EmitPod(ls1, p3)

	p4 := podbuilder.New(t, manifest2).
		WithPodID("pod4").
		WithPodLabel("baz", "quu").
		Build()
	f.kClient.EmitPod(ls2, p4)

	p5 := podbuilder.New(t, manifest2).
		WithPodID("pod5").
		Build()
	f.kClient.EmitPod(ls, p5)

	f.assertObservedPods(p4, p5)
	assert.Equal(t, []model.ManifestName{manifest2.Name, manifest2.Name}, f.manifestNames)
}

type podStatusTestCase struct {
	pod      corev1.PodStatus
	status   string
	messages []string
}

func TestPodStatus(t *testing.T) {
	cases := []podStatusTestCase{
		{
			pod: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 128,
								Message:  "failed to create containerd task: OCI runtime create failed: container_linux.go:345: starting container process caused \"exec: \\\"/hello\\\": stat /hello: no such file or directory\": unknown",
								Reason:   "StartError",
							},
						},
						Ready: false,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Message: "Back-off 40s restarting failed container=my-app pod=my-app-7bb79c789d-8h6n9_default(31369f71-df65-4352-b6bd-6d704a862699)",
								Reason:  "CrashLoopBackOff",
							},
						},
					},
				},
			},
			status: "CrashLoopBackOff",
			messages: []string{
				"failed to create containerd task: OCI runtime create failed: container_linux.go:345: starting container process caused \"exec: \\\"/hello\\\": stat /hello: no such file or directory\": unknown",
				"Back-off 40s restarting failed container=my-app pod=my-app-7bb79c789d-8h6n9_default(31369f71-df65-4352-b6bd-6d704a862699)",
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case%d", i), func(t *testing.T) {
			pod := corev1.Pod{Status: c.pod}
			status := PodStatusToString(pod)
			assert.Equal(t, c.status, status)

			messages := PodStatusErrorMessages(pod)
			assert.Equal(t, c.messages, messages)
		})
	}
}

func (f *pwFixture) addManifestWithSelectors(manifestName string, ls ...labels.Selector) model.Manifest {
	state := f.store.LockMutableStateForTesting()
	state.WatchFiles = true
	m := manifestbuilder.New(f, model.ManifestName(manifestName)).
		WithK8sYAML(testyaml.SanchoYAML).
		WithK8sPodSelectors(ls).
		Build()
	mt := store.NewManifestTarget(m)
	state.UpsertManifestTarget(mt)
	f.store.UnlockMutableState()
	return mt.Manifest
}

func (f *pwFixture) removeManifest(manifestName string) {
	mn := model.ManifestName(manifestName)
	state := f.store.LockMutableStateForTesting()
	delete(state.ManifestTargets, model.ManifestName(mn))
	var newDefOrder []model.ManifestName
	for _, e := range state.ManifestDefinitionOrder {
		if mn != e {
			newDefOrder = append(newDefOrder, e)
		}
	}
	state.ManifestDefinitionOrder = newDefOrder
	f.store.UnlockMutableState()
}

type pwFixture struct {
	*tempdir.TempDirFixture
	t             *testing.T
	kClient       *k8s.FakeK8sClient
	pw            *PodWatcher
	ctx           context.Context
	cancel        func()
	store         *store.Store
	pods          []*corev1.Pod
	manifestNames []model.ManifestName
}

func (pw *pwFixture) reducer(ctx context.Context, state *store.EngineState, action store.Action) {
	a, ok := action.(PodChangeAction)
	if !ok {
		pw.t.Errorf("Expected action type PodLogAction. Actual: %T", action)
	}
	pw.pods = append(pw.pods, a.Pod)
	pw.manifestNames = append(pw.manifestNames, a.ManifestName)
}

func newPWFixture(t *testing.T) *pwFixture {
	kClient := k8s.NewFakeK8sClient()

	ctx, _, _ := testutils.CtxAndAnalyticsForTest()
	ctx, cancel := context.WithCancel(ctx)

	of := k8s.ProvideOwnerFetcher(kClient)
	pw := NewPodWatcher(kClient, of)
	ret := &pwFixture{
		TempDirFixture: tempdir.NewTempDirFixture(t),
		kClient:        kClient,
		pw:             pw,
		ctx:            ctx,
		cancel:         cancel,
		t:              t,
	}

	st := store.NewStore(store.Reducer(ret.reducer), store.LogActionsFlag(false))
	go st.Loop(ctx)

	ret.store = st

	return ret
}

func (f *pwFixture) TearDown() {
	f.TempDirFixture.TearDown()
	f.kClient.TearDown()
	f.cancel()
}

func (f *pwFixture) addDeployedUID(m model.Manifest, uid types.UID) {
	defer f.pw.OnChange(f.ctx, f.store)

	state := f.store.LockMutableStateForTesting()
	defer f.store.UnlockMutableState()
	mState, ok := state.ManifestState(m.Name)
	if !ok {
		f.t.Fatalf("Unknown manifest: %s", m.Name)
	}
	runtimeState := mState.GetOrCreateK8sRuntimeState()
	runtimeState.DeployedUIDSet[uid] = true
}

func (f *pwFixture) assertObservedPods(pods ...*corev1.Pod) {
	start := time.Now()
	for time.Since(start) < 200*time.Millisecond {
		if len(pods) == len(f.pods) {
			break
		}
	}

	if !assert.ElementsMatch(f.t, pods, f.pods) {
		f.t.FailNow()
	}
}

func (f *pwFixture) assertWatchedSelectors(ls ...labels.Selector) {
	start := time.Now()
	for time.Since(start) < 200*time.Millisecond {
		if len(ls) == len(f.kClient.WatchedSelectors()) {
			break
		}
	}

	if !assert.ElementsMatch(f.t, ls, f.kClient.WatchedSelectors()) {
		f.t.FailNow()
	}
}

func (f *pwFixture) clearPods() {
	f.pods = nil
	f.manifestNames = nil
}
