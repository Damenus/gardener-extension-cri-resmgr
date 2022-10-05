// Copyright 2022 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"fmt"
	"time"

	// Local
	"github.com/intel/gardener-extension-cri-resmgr/pkg/consts"
	"github.com/intel/gardener-extension-cri-resmgr/pkg/controller/healthcheck"
	actuator "github.com/intel/gardener-extension-cri-resmgr/pkg/controller/lifecycle"

	// Gardener
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	resourcemanagerv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"

	// Other
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func NewExtensionControllerCommand(ctx context.Context) *cobra.Command {

	options := NewOptions()

	cmd := &cobra.Command{
		Use:   "cri-resmgr-controller-manager",
		Short: "CRI Resource manager Controller manages components which install CRI-Resource-Manager as CRI proxy.",

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.optionAggregator.Complete(); err != nil {
				return fmt.Errorf("error completing options: %s", err)
			}

			mgrOpts := manager.Options{
				LeaderElection:     false,
				MetricsBindAddress: "0",
			}

			mgr, err := manager.New(options.restOptions.Completed().Config, mgrOpts)
			if err != nil {
				return fmt.Errorf("could not instantiate controller-manager: %s", err)
			}
			scheme := mgr.GetScheme()
			if err := extensionscontroller.AddToScheme(scheme); err != nil {
				return err
			}
			if err := resourcemanagerv1alpha1.AddToScheme(scheme); err != nil {
				return err
			}

			// Enable healthcheck.
			// "Registration" adds additional controller that watches over Extension/Cluster.
			if err := healthcheck.RegisterHealthChecks(mgr); err != nil {
				return err
			}

			ignoreOperationAnnotation := options.reconcileOptions.Completed().IgnoreOperationAnnotation
			// if true:
			//		predicates: only observe "generation change" predicate (oldObject.generation != newObject.generation)
			// 		watches:  watch Cluster (additionally and map to extensions) and Extension
			//
			// if false (default):
			//      predicates: (defaultControllerPredicates) watches for "operation annotation" to be reconcile/migrate/restore
			//					or deletionTimestamp is set or lastOperation is not successful state (on Extension object)
			// 		watches: only Extension
			log.Log.Info("Reconciler options", "ignoreOperationAnnotation", ignoreOperationAnnotation)

			if err := extension.Add(mgr, extension.AddArgs{
				Actuator:                  actuator.NewActuator(),
				ControllerOptions:         options.controllerOptions.Completed().Options(),
				Name:                      consts.ControllerName,
				FinalizerSuffix:           consts.ExtensionType,
				Resync:                    60 * time.Minute,
				Type:                      consts.ExtensionType, // to be used for TypePredicate
				Predicates:                extension.DefaultPredicates(ignoreOperationAnnotation),
				IgnoreOperationAnnotation: ignoreOperationAnnotation,
			}); err != nil {
				return fmt.Errorf("error configuring controller with extensions actuator: %s", err)
			}

			if err := mgr.Start(ctx); err != nil {
				return fmt.Errorf("error running manager: %s", err)
			}

			return nil
		},
	}

	options.optionAggregator.AddFlags(cmd.Flags())

	return cmd
}