/*
Copyright 2022-2024 EscherCloud.
Copyright 2024-2025 the Unikorn Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"text/template"
	"time"

	"github.com/spf13/pflag"

	"github.com/unikorn-cloud/core/pkg/util/retry"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	// metalLBVersion is the version of the loabalancer controller to
	// install.
	metalLBVersion = "v0.13.5"

	// metalLBManifest describes where to get the installer manifest from.
	// This will create a namespace 'metalLB-system' and all the other bits
	// in there.  There will be a deployment called 'controller' we need to
	// wait to become available, and a daemonset called 'speaker' that does
	// all of the routing goodies that also need to become available.
	metalLBManifest = "https://raw.githubusercontent.com/metallb/metallb/" + metalLBVersion + "/config/manifests/metallb-native.yaml"

	// metalLBNamespace is where metalLB goes by default.
	metalLBNamespace = "metallb-system"

	// matallbAddressPoolTemplate is a bunch of CR configuration to set the
	// VIP address ranges.
	matallbAddressPoolTemplate = `apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: example
  namespace: metallb-system
spec:
  addresses:
  - {{.start}}-{{.end}}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: empty
  namespace: metallb-system
`
)

var (
	// ErrConditionFormat means the formatting of the condition is wrong,
	// these are loosly defined, but there are some conventions.
	ErrConditionFormat = errors.New("status condition incorrectly formatted")

	// ErrConditionMissing means the condition isn't present.
	ErrConditionMissing = errors.New("status condition not found")

	// ErrConditionStatus means the condition has the wrong truthiness.
	ErrConditionStatus = errors.New("status condition incorrect status")

	ErrDaemonSetUnready = errors.New("daemonset readiness doesn't match desired")
)

// waitCondition waits for a condtion to be true on a generic resource.
func waitCondition(ctx context.Context, client dynamic.Interface, group, version, resource, namespace, name, conditionType string) {
	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	callback := func() error {
		object, err := client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		conditions, _, err := unstructured.NestedSlice(object.Object, "status", "conditions")
		if err != nil {
			return fmt.Errorf("%w: conditions lookup error: %s", ErrConditionFormat, err.Error())
		}

		for i := range conditions {
			condition, ok := conditions[i].(map[string]any)
			if !ok {
				return fmt.Errorf("%w: condition type assertion error", ErrConditionFormat)
			}

			t, _, err := unstructured.NestedString(condition, "type")
			if err != nil {
				return fmt.Errorf("%w: condition type error: %s", ErrConditionFormat, err.Error())
			}

			if t != conditionType {
				continue
			}

			s, _, err := unstructured.NestedString(condition, "status")
			if err != nil {
				return fmt.Errorf("%w: condition status error: %s", ErrConditionFormat, err.Error())
			}

			if s != "True" {
				return ErrConditionStatus
			}

			return nil
		}

		return ErrConditionMissing
	}

	if err := retry.Forever().DoWithContext(ctx, callback); err != nil {
		panic(err)
	}
}

// waitDaemonSetReady performs a type specific wait function until the desired and actual
// number of rready processes match.
func waitDaemonSetReady(ctx context.Context, client kubernetes.Interface, namespace, name string) {
	callback := func() error {
		daemonset, err := client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("daemonset get error: %w", err)
		}

		if daemonset.Status.NumberReady != daemonset.Status.DesiredNumberScheduled {
			return fmt.Errorf("%w: status mismatch", ErrDaemonSetUnready)
		}

		return nil
	}

	if err := retry.Forever().DoWithContext(ctx, callback); err != nil {
		panic(err)
	}
}

func provision(config *genericclioptions.ConfigFlags, path string) error {
	var args []string

	// If explcitly specified in the top level command, use these
	if config.KubeConfig != nil && len(*config.KubeConfig) > 0 {
		args = append(args, "--kubeconfig", *config.KubeConfig)
	}

	if config.Context != nil && len(*config.Context) > 0 {
		args = append(args, "--context", *config.Context)
	}

	args = append(args, "apply", "-f", path)

	if err := exec.Command("kubectl", args...).Run(); err != nil {
		return err
	}

	return nil
}

// applyManifest does exactly that.
func applyManifest(config *genericclioptions.ConfigFlags, path string) {
	if err := provision(config, path); err != nil {
		panic(err)
	}
}

// getDockerNetwork is a utility function to derive the IPv4 network from the
// specified Kind cluster.  Anything in this prefix will be routable from the
// host.
func getDockerNetwork(name string) *net.IPNet {
	out, err := exec.Command("docker", "network", "inspect", name).Output()
	if err != nil {
		panic(err)
	}

	var dockerNetConfigs []map[string]any

	if err := json.Unmarshal(out, &dockerNetConfigs); err != nil {
		panic(err)
	}

	if len(dockerNetConfigs) != 1 {
		panic("wrong net config length")
	}

	ipamConfigs, _, err := unstructured.NestedSlice(dockerNetConfigs[0], "IPAM", "Config")
	if err != nil {
		panic(err)
	}

	for i := range ipamConfigs {
		ipamConfig, ok := ipamConfigs[i].(map[string]any)
		if !ok {
			panic("config format fail")
		}

		prefix, _, err := unstructured.NestedString(ipamConfig, "Subnet")
		if err != nil {
			panic("subnet fail")
		}

		_, network, err := net.ParseCIDR(prefix)
		if err != nil {
			panic(err)
		}

		v4 := network.IP.To4()
		if v4 == nil {
			continue
		}

		return network
	}

	panic("no IPv4 subnet found")
}

// getVIPRange is, quite frankly, a hack that allocates an address range from a CIDR
// in the vain hope that said range will not be allocated by anything else.  To that
// end it picks the last possible /24 range and carves a bit out of that.
func getVIPRange(network *net.IPNet, rangeStart, rangeEnd uint) (net.IP, net.IP) {
	v4 := network.IP.To4()

	// Convert the IPv4 prefix to an unsigned integer e.g. 172.18.0.0/16 -> 0xac120000.
	v4int := uint(v4[0])<<24 | uint(v4[1])<<16 | uint(v4[2])<<8 | uint(v4[3])

	// Calculate the topmost /24 e.g. (1<<(32-16))-1 -> 0xffff & 0xffffff00 -> 0xff00.
	ones, bits := network.Mask.Size()
	offset := ((1 << (bits - ones)) - 1) & ^uint(0xff)

	// Add the offset to the prefix, and some start and end ranges
	// e.g. 0xac120000 + 0xff00 + 0xc8 -> 0xac12ffc8.
	v4VIPPrefix := v4int + offset
	v4start := v4VIPPrefix + rangeStart
	v4end := v4VIPPrefix + rangeEnd

	// And finally convert pack into internal types.
	start := net.IPv4(byte(v4start>>24), byte(v4start>>16), byte(v4start>>8), byte(v4start))
	end := net.IPv4(byte(v4end>>24), byte(v4end>>16), byte(v4end>>8), byte(v4end))

	return start, end
}

// applyMetalLBAddressPools creates a couple MetalLB custom resources that define an address
// pool for external connectivity and L2 shizzle that will respond to ARP whohas requests
// and take ownership.
func applyMetalLBAddressPools(config *genericclioptions.ConfigFlags, start, end net.IP) {
	tmpl := template.New("foo")

	if _, err := tmpl.Parse(matallbAddressPoolTemplate); err != nil {
		panic(err)
	}

	tf, err := os.CreateTemp("", "")
	if err != nil {
		panic(err)
	}

	defer os.Remove(tf.Name())

	ctx := map[string]string{
		"start": start.String(),
		"end":   end.String(),
	}

	if err := tmpl.Execute(tf, ctx); err != nil {
		panic(err)
	}

	tf.Close()

	applyManifest(config, tf.Name())
}

// main is the main entry point, shock!
// It will install (idempotently) MetalLB, figure out an IP address range to provision
// load balancer VIPs from, and make that live.  For a real cloud this is a non-event,
// this is more for local testing with Kind and other provisioners of that ilk.
func main() {
	// Parse flags.
	var clusterName string

	var timeout time.Duration

	pflag.StringVar(&clusterName, "cluster-name", "kind", "Kind cluster name to probe.")
	pflag.DurationVar(&timeout, "timeout", 5*time.Minute, "Global timeout to complete installation.")

	configFlags := genericclioptions.NewConfigFlags(true)
	configFlags.AddFlags(pflag.CommandLine)

	pflag.Parse()

	// Perform Kubernetes configuration.
	config, err := configFlags.ToRESTConfig()
	if err != nil {
		panic(err)
	}

	kubernetesClient := kubernetes.NewForConfigOrDie(config)
	dynamicClient := dynamic.NewForConfigOrDie(config)

	// Set up our global timeout.
	c, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	// And finally do the install.
	fmt.Println("🦄 Applying MetalLB manifest ...")
	applyManifest(configFlags, metalLBManifest)

	fmt.Println("🦄 Waiting for MetalLB controller to be ready ...")
	waitCondition(c, dynamicClient, "apps", "v1", "deployments", metalLBNamespace, "controller", "Available")

	fmt.Println("🦄 Waiting for MetalLB daemonset to be ready ...")
	waitDaemonSetReady(c, kubernetesClient, metalLBNamespace, "speaker")

	fmt.Println("🦄 Getting network configuration ...")

	network := getDockerNetwork(clusterName)
	fmt.Println("💡 Using routable prefix", network)

	start, end := getVIPRange(network, 200, 250)
	fmt.Println("💡 Using address range", start, "-", end)

	fmt.Println("🦄 Applying MetalLB network configuration ...")
	applyMetalLBAddressPools(configFlags, start, end)
}
