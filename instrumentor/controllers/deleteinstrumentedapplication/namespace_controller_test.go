package deleteinstrumentedapplication_test

import (
	"context"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/instrumentor/internal/testutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("DeleteKarmaInstrumentedApplication Namespace controller", func() {

	ctx := context.Background()
	var namespace *corev1.Namespace

	var deployment *appsv1.Deployment
	var daemonSet *appsv1.DaemonSet
	var statefulSet *appsv1.StatefulSet

	var instrumentedApplicationDeployment *odigosv1.KarmaInstrumentedApplication
	var instrumentedApplicationDaemonSet *odigosv1.KarmaInstrumentedApplication
	var instrumentedApplicationStatefulSet *odigosv1.KarmaInstrumentedApplication

	When("namespace instrumentation is disabled", func() {

		BeforeEach(func() {
			namespace = testutil.SetOdigosInstrumentationEnabled(testutil.NewMockNamespace())
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
		})

		Context("workloads instrumentation label is not set (inherit from namespace)", func() {

			BeforeEach(func() {
				deployment = testutil.SetReportedNameAnnotation(testutil.NewMockTestDeployment(namespace), "foo")
				Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())
				daemonSet = testutil.SetReportedNameAnnotation(testutil.NewMockTestDaemonSet(namespace), "foo")
				Expect(k8sClient.Create(ctx, daemonSet)).Should(Succeed())
				statefulSet = testutil.SetReportedNameAnnotation(testutil.NewMockTestStatefulSet(namespace), "foo")
				Expect(k8sClient.Create(ctx, statefulSet)).Should(Succeed())

				// these workloads has instrumentation application because the namespace has instrumentation enabled
				instrumentedApplicationDeployment = testutil.NewMockKarmaInstrumentedApplication(deployment)
				Expect(k8sClient.Create(ctx, instrumentedApplicationDeployment)).Should(Succeed())
				instrumentedApplicationDaemonSet = testutil.NewMockKarmaInstrumentedApplication(daemonSet)
				Expect(k8sClient.Create(ctx, instrumentedApplicationDaemonSet)).Should(Succeed())
				instrumentedApplicationStatefulSet = testutil.NewMockKarmaInstrumentedApplication(statefulSet)
				Expect(k8sClient.Create(ctx, instrumentedApplicationStatefulSet)).Should(Succeed())
			})

			It("should delete instrumented application", func() {

				namespace = testutil.SetOdigosInstrumentationDisabled(namespace)
				Expect(k8sClient.Update(ctx, namespace)).Should(Succeed())

				testutil.AssertKarmaInstrumentedApplicationDeleted(ctx, k8sClient, instrumentedApplicationDeployment)
				testutil.AssertKarmaInstrumentedApplicationDeleted(ctx, k8sClient, instrumentedApplicationDaemonSet)
				testutil.AssertKarmaInstrumentedApplicationDeleted(ctx, k8sClient, instrumentedApplicationStatefulSet)
			})

			It("should delete reported name annotation", func() {

				namespace = testutil.SetOdigosInstrumentationDisabled(namespace)
				Expect(k8sClient.Update(ctx, namespace)).Should(Succeed())

				testutil.AssertReportedNameAnnotationDeletedDeployment(ctx, k8sClient, deployment)
				testutil.AssertReportedNameAnnotationDeletedDaemonSet(ctx, k8sClient, daemonSet)
				testutil.AssertReportedNameAnnotationDeletedStatefulSet(ctx, k8sClient, statefulSet)
			})
		})

		Context("workloads instrumentation label enabled (override namespace)", func() {

			BeforeEach(func() {
				deployment = testutil.SetOdigosInstrumentationEnabled(testutil.NewMockTestDeployment(namespace))
				Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())
				daemonSet = testutil.SetOdigosInstrumentationEnabled(testutil.NewMockTestDaemonSet(namespace))
				Expect(k8sClient.Create(ctx, daemonSet)).Should(Succeed())
				statefulSet = testutil.SetOdigosInstrumentationEnabled(testutil.NewMockTestStatefulSet(namespace))
				Expect(k8sClient.Create(ctx, statefulSet)).Should(Succeed())

				// these workloads has instrumentation application because the namespace has instrumentation enabled
				instrumentedApplicationDeployment = testutil.NewMockKarmaInstrumentedApplication(deployment)
				Expect(k8sClient.Create(ctx, instrumentedApplicationDeployment)).Should(Succeed())
				instrumentedApplicationDaemonSet = testutil.NewMockKarmaInstrumentedApplication(daemonSet)
				Expect(k8sClient.Create(ctx, instrumentedApplicationDaemonSet)).Should(Succeed())
				instrumentedApplicationStatefulSet = testutil.NewMockKarmaInstrumentedApplication(statefulSet)
				Expect(k8sClient.Create(ctx, instrumentedApplicationStatefulSet)).Should(Succeed())
			})

			It("should retain instrumented application", func() {
				namespace = testutil.SetOdigosInstrumentationDisabled(namespace)
				Expect(k8sClient.Update(ctx, namespace)).Should(Succeed())

				testutil.AssertKarmaInstrumentedApplicationRetained(ctx, k8sClient, instrumentedApplicationDeployment)
				testutil.AssertKarmaInstrumentedApplicationRetained(ctx, k8sClient, instrumentedApplicationDaemonSet)
				testutil.AssertKarmaInstrumentedApplicationRetained(ctx, k8sClient, instrumentedApplicationStatefulSet)
			})

		})

	})
})
