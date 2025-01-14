package engine

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	tiltanalytics "github.com/windmilleng/tilt/internal/analytics"
	"github.com/windmilleng/tilt/internal/container"

	"github.com/windmilleng/wmclient/pkg/analytics"

	"github.com/windmilleng/tilt/internal/store"
	"github.com/windmilleng/tilt/pkg/model"
)

var (
	fb = model.FastBuild{HotReload: true}                                            // non-empty FastBuild
	lu = model.LiveUpdate{Steps: []model.LiveUpdateStep{model.LiveUpdateSyncStep{}}} // non-empty LiveUpdate

	imgTargDB       = model.ImageTarget{BuildDetails: model.DockerBuild{}}
	imgTargFB       = model.ImageTarget{BuildDetails: fb}
	imgTargDBWithFB = model.ImageTarget{BuildDetails: model.DockerBuild{FastBuild: fb}}
	imgTargDBWithLU = model.ImageTarget{BuildDetails: model.DockerBuild{LiveUpdate: lu}}

	kTarg = model.K8sTarget{}
	dTarg = model.DockerComposeTarget{}
)

var (
	r1 = "gcr.io/some-project-162817/one"
	r2 = "gcr.io/some-project-162817/two"
	r3 = "gcr.io/some-project-162817/three"
	r4 = "gcr.io/some-project-162817/four"

	iTargWithRef1     = iTargetForRef(r1).WithBuildDetails(model.DockerBuild{LiveUpdate: lu})
	iTargWithRef2     = iTargetForRef(r2).WithBuildDetails(model.DockerBuild{LiveUpdate: lu})
	iTargWithRef3     = iTargetForRef(r3).WithBuildDetails(model.DockerBuild{LiveUpdate: lu})
	iTargWithRef4NoLU = iTargetForRef(r4)
)

func TestAnalyticsReporter_Everything(t *testing.T) {
	tf := newAnalyticsReporterTestFixture()

	tf.addManifest(tf.nextManifest().WithImageTarget(imgTargFB))                               // fastbuild
	tf.addManifest(tf.nextManifest().WithImageTarget(imgTargDB).WithDeployTarget(kTarg))       // k8s
	tf.addManifest(tf.nextManifest().WithImageTarget(imgTargDBWithFB))                         // anyfastbuild
	tf.addManifest(tf.nextManifest().WithImageTarget(imgTargDBWithLU))                         // liveupdate
	tf.addManifest(tf.nextManifest().WithDeployTarget(kTarg))                                  // k8s, unbuilt
	tf.addManifest(tf.nextManifest().WithDeployTarget(kTarg))                                  // k8s, unbuilt
	tf.addManifest(tf.nextManifest().WithDeployTarget(kTarg))                                  // k8s, unbuilt
	tf.addManifest(tf.nextManifest().WithDeployTarget(dTarg))                                  // dc
	tf.addManifest(tf.nextManifest().WithDeployTarget(dTarg))                                  // dc
	tf.addManifest(tf.nextManifest().WithImageTarget(imgTargDBWithLU).WithDeployTarget(dTarg)) // dc, liveupdate
	tf.addManifest(tf.nextManifest().WithImageTargets(
		[]model.ImageTarget{imgTargDBWithLU, imgTargDBWithLU})) // liveupdate, multipleimageliveupdate

	state := tf.ar.store.LockMutableStateForTesting()
	state.TiltStartTime = time.Now()

	state.CompletedBuildCount = 3

	tf.ar.store.UnlockMutableState()

	tf.run()

	expectedTags := map[string]string{
		"builds.completed_count":                              "3",
		"resource.count":                                      "11",
		"resource.dockercompose.count":                        "3",
		"resource.unbuiltresources.count":                     "3",
		"resource.fastbuild.count":                            "1",
		"resource.anyfastbuild.count":                         "2",
		"resource.liveupdate.count":                           "3",
		"resource.k8s.count":                                  "4",
		"resource.sameimagemultiplecontainerliveupdate.count": "0", // tests for this below
		"resource.multipleimageliveupdate.count":              "1",
		"tiltfile.error":                                      "false",
		"up.starttime":                                        state.TiltStartTime.Format(time.RFC3339),
	}

	tf.assertStats(t, expectedTags)
}

func TestAnalyticsReporter_SameImageMultiContainer(t *testing.T) {
	tf := newAnalyticsReporterTestFixture()

	injectCountsA := map[string]int{
		r1: 1,
		r2: 2,
	}
	k8sTargA := kTarg.WithRefInjectCounts(injectCountsA)
	tf.addManifest(tf.nextManifest().
		WithImageTarget(iTargWithRef1).
		WithImageTarget(iTargWithRef2).
		WithDeployTarget(k8sTargA))

	injectCountsB := map[string]int{
		r2: 2,
		r3: 3,
	}
	k8sTargB := kTarg.WithRefInjectCounts(injectCountsB)
	tf.addManifest(tf.nextManifest().
		WithImageTarget(iTargWithRef2).
		WithImageTarget(iTargWithRef3).
		WithDeployTarget(k8sTargB))

	tf.run()

	assert.Equal(t, "2", tf.ma.Counts[0].Tags["resource.sameimagemultiplecontainerliveupdate.count"])
}

func TestAnalyticsReporter_SameImageMultiContainer_NoIncr(t *testing.T) {
	tf := newAnalyticsReporterTestFixture()

	injectCounts := map[string]int{
		r1: 1,
		r4: 2,
	}
	k8sTarg := kTarg.WithRefInjectCounts(injectCounts)
	tf.addManifest(tf.nextManifest().
		WithImageTarget(iTargWithRef1).
		WithImageTarget(iTargWithRef4NoLU). // injects multiple times, but no LU so won't record stat for it
		WithDeployTarget(k8sTarg))

	tf.run()

	assert.Equal(t, "0", tf.ma.Counts[0].Tags["resource.sameimagemultiplecontainerliveupdate.count"])
}

func TestAnalyticsReporter_TiltfileError(t *testing.T) {
	tf := newAnalyticsReporterTestFixture()

	tf.addManifest(tf.nextManifest().WithImageTarget(model.ImageTarget{BuildDetails: model.FastBuild{}}))
	tf.addManifest(tf.nextManifest().WithImageTarget(model.ImageTarget{BuildDetails: model.DockerBuild{}}))
	tf.addManifest(tf.nextManifest().WithDeployTarget(model.K8sTarget{}))
	tf.addManifest(tf.nextManifest().WithDeployTarget(model.K8sTarget{}))
	tf.addManifest(tf.nextManifest().WithDeployTarget(model.K8sTarget{}))
	tf.addManifest(tf.nextManifest().WithDeployTarget(model.DockerComposeTarget{}))
	tf.addManifest(tf.nextManifest().WithDeployTarget(model.DockerComposeTarget{}))
	tf.addManifest(tf.nextManifest().WithDeployTarget(model.DockerComposeTarget{}))
	tf.addManifest(tf.nextManifest().WithDeployTarget(model.DockerComposeTarget{}))

	state := tf.ar.store.LockMutableStateForTesting()
	state.TiltStartTime = time.Now()

	state.CompletedBuildCount = 3

	state.TiltfileState.AddCompletedBuild(model.BuildRecord{Error: errors.New("foo")})

	tf.ar.store.UnlockMutableState()

	tf.run()

	expectedTags := map[string]string{
		"builds.completed_count": "3",
		"tiltfile.error":         "true",
		"up.starttime":           state.TiltStartTime.Format(time.RFC3339),
	}

	tf.assertStats(t, expectedTags)
}

type analyticsReporterTestFixture struct {
	manifestCount int
	ar            AnalyticsReporter
	ma            *analytics.MemoryAnalytics
}

func newAnalyticsReporterTestFixture() *analyticsReporterTestFixture {
	st, _ := store.NewStoreForTesting()
	ma, a := tiltanalytics.NewMemoryTiltAnalyticsForTest(tiltanalytics.NullOpter{})
	ar := AnalyticsReporter{
		a:       a,
		store:   st,
		started: false,
	}

	return &analyticsReporterTestFixture{
		manifestCount: 0,
		ar:            ar,
		ma:            ma,
	}
}

func (artf *analyticsReporterTestFixture) addManifest(m model.Manifest) {
	state := artf.ar.store.LockMutableStateForTesting()
	state.UpsertManifestTarget(store.NewManifestTarget(m))
	artf.ar.store.UnlockMutableState()
}

func (artf *analyticsReporterTestFixture) nextManifest() model.Manifest {
	artf.manifestCount++
	return model.Manifest{Name: model.ManifestName(fmt.Sprintf("manifest%d", artf.manifestCount))}
}

func (artf *analyticsReporterTestFixture) run() {
	artf.ar.report()

	artf.ar.a.Flush(500 * time.Second)
}

func (artf *analyticsReporterTestFixture) assertStats(t *testing.T, expectedTags map[string]string) {
	expectedCounts := []analytics.CountEvent{{Name: "up.running", N: 1, Tags: expectedTags}}
	assert.Equal(t, expectedCounts, artf.ma.Counts)
}

func iTargetForRef(ref string) model.ImageTarget {
	named := container.MustParseNamed(ref)
	selector := container.NameSelector(named)
	return model.NewImageTarget(selector)
}
