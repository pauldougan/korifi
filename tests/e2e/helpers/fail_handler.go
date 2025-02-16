package helpers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

const (
	logTailLines = 50
)

type podContainerDescriptor struct {
	Namespace     string
	LabelKey      string
	LabelValue    string
	Container     string
	CorrelationId string
}

func E2EFailHandler(correlationId func() string) func(string, ...int) {
	return func(message string, callerSkip ...int) {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Failure correlation ID: %q\n", correlationId())
		config, err := controllerruntime.GetConfig()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		clientset, err := kubernetes.NewForConfig(config)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		printPodsLogs(clientset, []podContainerDescriptor{
			{
				Namespace:     "korifi-api-system",
				LabelKey:      "app",
				LabelValue:    "korifi-api",
				Container:     "korifi-api",
				CorrelationId: correlationId(),
			},
			{
				Namespace:  "korifi-controllers-system",
				LabelKey:   "control-plane",
				LabelValue: "controller-manager",
				Container:  "manager",
			},
		})

		if len(callerSkip) > 0 {
			callerSkip[0] = callerSkip[0] + 1
		} else {
			callerSkip = []int{1}
		}

		ginkgo.Fail(message, callerSkip...)
	}
}

func printPodsLogs(clientset kubernetes.Interface, podContainerDescriptors []podContainerDescriptor) {
	for _, desc := range podContainerDescriptors {
		pods, err := getPods(clientset, desc.Namespace, desc.LabelKey, desc.LabelValue)
		if err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get pods with label %s=%s: %v\n", desc.LabelKey, desc.LabelValue, err)
		}

		for _, pod := range pods {
			log, err := getSinglePodLog(clientset, pod, desc.Container, desc.CorrelationId)
			if err != nil {
				fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get logs for pod %s: %v\n", pod.Name, err)

				continue
			}
			if log == "" {
				log = "No relevant logs found"
			}

			logHeader := fmt.Sprintf(
				"Logs for pod %q (last %d lines)",
				pod.Name,
				logTailLines,
			)
			if desc.CorrelationId != "" {
				logHeader = fmt.Sprintf(
					"Logs for pod %q with correlation ID %q (last %d lines)",
					pod.Name,
					desc.CorrelationId,
					logTailLines,
				)
			}

			fmt.Fprintf(ginkgo.GinkgoWriter,
				"\n\n===== %s =====\n%s\n==============================================\n\n",
				logHeader,
				log)
		}
	}
}

func getPods(clientset kubernetes.Interface, namespace, labelKey, labelValue string) ([]corev1.Pod, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", labelKey, labelValue),
	})
	if err != nil {
		return nil, err
	}

	return pods.Items, nil
}

func getSinglePodLog(clientset kubernetes.Interface, pod corev1.Pod, container, correlationId string) (string, error) {
	podLogOpts := corev1.PodLogOptions{TailLines: int64ptr(logTailLines), Container: container}
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)

	logStream, err := req.Stream(context.Background())
	if err != nil {
		return "", err
	}
	defer logStream.Close()

	var logBuf bytes.Buffer
	logScanner := bufio.NewScanner(logStream)

	for logScanner.Scan() {
		if strings.Contains(logScanner.Text(), correlationId) {
			logBuf.WriteString(logScanner.Text() + "\n")
		}
	}

	return logBuf.String(), logScanner.Err()
}

func int64ptr(i int64) *int64 {
	return &i
}
