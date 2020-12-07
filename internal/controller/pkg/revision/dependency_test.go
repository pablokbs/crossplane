/*
Copyright 2020 The Crossplane Authors.

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

package revision

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/crossplane/crossplane-runtime/pkg/test"

	pkgmeta "github.com/crossplane/crossplane/apis/pkg/meta/v1alpha1"
	"github.com/crossplane/crossplane/apis/pkg/v1alpha1"
	"github.com/crossplane/crossplane/apis/pkg/v1beta1"
	"github.com/crossplane/crossplane/internal/dag"
	dagfake "github.com/crossplane/crossplane/internal/dag/fake"
)

var _ DependencyManager = &PackageDependencyManager{}

func TestResolve(t *testing.T) {
	errBoom := errors.New("boom")

	type args struct {
		dep  *PackageDependencyManager
		meta runtime.Object
		pr   v1beta1.PackageRevision
	}

	type want struct {
		err       error
		total     int
		installed int
		invalid   int
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ErrNotMeta": {
			reason: "Should return error if not a valid package meta type.",
			args: args{
				dep:  &PackageDependencyManager{},
				meta: &v1beta1.Configuration{},
			},
			want: want{
				err: errors.New(errNotMeta),
			},
		},
		"ErrGetLock": {
			reason: "Should return error if we cannot get lock.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(errBoom),
					},
				},
				meta: &pkgmeta.Configuration{},
			},
			want: want{
				err: errors.Wrap(errBoom, errGetLock),
			},
		},
		"ErrBuildDag": {
			reason: "Should return error if we cannot build DAG.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(_ []dag.Node, _ ...dag.NodeFn) ([]dag.Node, error) {
								return nil, errBoom
							},
						}
					},
				},
				meta: &pkgmeta.Configuration{},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package: "hasheddan/config-nop-a:v0.0.1",
					},
				},
			},
			want: want{
				err: errBoom,
			},
		},
		"SuccessfulInactiveAlreadyRemoved": {
			reason: "Should not return error if we are inactive and not in lock.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(_ []dag.Node, _ ...dag.NodeFn) ([]dag.Node, error) {
								return nil, nil
							},
						}
					},
				},
				meta: &pkgmeta.Configuration{},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package:      "hasheddan/config-nop-a:v0.0.1",
						DesiredState: v1beta1.PackageRevisionInactive,
					},
				},
			},
			want: want{
				err: nil,
			},
		},
		"SuccessfulInactiveExists": {
			reason: "Should not return error if we are inactive and not in lock.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
							l := obj.(*v1alpha1.Lock)
							l.Packages = []v1alpha1.LockPackage{
								{
									Source: "hasheddan/config-nop-a:v0.0.1",
								},
							}
							return nil
						}),
						MockUpdate: test.NewMockUpdateFn(nil),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(nodes []dag.Node, fns ...dag.NodeFn) ([]dag.Node, error) {
								for i, n := range nodes {
									for _, f := range fns {
										f(i, n)
									}
								}
								return nil, nil
							},
						}
					},
				},
				meta: &pkgmeta.Configuration{},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package:      "hasheddan/config-nop-a:v0.0.1",
						DesiredState: v1beta1.PackageRevisionInactive,
					},
				},
			},
			want: want{
				err: nil,
			},
		},
		"ErrorRemoveInactiveFromLock": {
			reason: "Should return error if we are inactive and fail to remove from lock.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
							l := obj.(*v1alpha1.Lock)
							l.Packages = []v1alpha1.LockPackage{
								{
									Source: "hasheddan/config-nop-a",
								},
							}
							return nil
						}),
						MockUpdate: test.NewMockUpdateFn(errBoom),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(nodes []dag.Node, fns ...dag.NodeFn) ([]dag.Node, error) {
								for i, n := range nodes {
									for _, f := range fns {
										f(i, n)
									}
								}
								return nil, nil
							},
						}
					},
				},
				meta: &pkgmeta.Configuration{},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package:      "hasheddan/config-nop-a:v0.0.1",
						DesiredState: v1beta1.PackageRevisionInactive,
					},
				},
			},
			want: want{
				err: errBoom,
			},
		},
		"SuccessfulSelfExistNoDependencies": {
			reason: "Should not return error if self exists and has no dependencies.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
							l := obj.(*v1alpha1.Lock)
							l.Packages = []v1alpha1.LockPackage{
								{
									Source: "hasheddan/config-nop-a",
								},
							}
							return nil
						}),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(nodes []dag.Node, fns ...dag.NodeFn) ([]dag.Node, error) {
								for i, n := range nodes {
									for _, f := range fns {
										f(i, n)
									}
								}
								return nil, nil
							},
							MockTraceNode: func(_ string) (map[string]dag.Node, error) {
								return nil, nil
							},
						}
					},
				},
				meta: &pkgmeta.Configuration{},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package:      "hasheddan/config-nop-a:v0.0.1",
						DesiredState: v1beta1.PackageRevisionActive,
					},
				},
			},
			want: want{},
		},
		"ErrorSelfNotExistMissingDirectDependencies": {
			reason: "Should return error if self does not exist and missing direct dependencies.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
							l := obj.(*v1alpha1.Lock)
							l.Packages = []v1alpha1.LockPackage{
								{
									Source: "hasheddan/config-nop-a",
									Dependencies: []v1alpha1.Dependency{
										{
											Package: "not-here-1",
											Type:    v1alpha1.ProviderPackageType,
										},
										{
											Package: "not-here-2",
											Type:    v1alpha1.ConfigurationPackageType,
										},
									},
								},
							}
							return nil
						}),
						MockUpdate: test.NewMockUpdateFn(nil),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(nodes []dag.Node, fns ...dag.NodeFn) ([]dag.Node, error) {
								return nil, nil
							},
							MockAddNode: func(_ dag.Node) error {
								return nil
							},
							MockNodeExists: func(_ string) bool {
								return false
							},
							MockAddOrUpdateNodes: func(_ ...dag.Node) {},
						}
					},
				},
				meta: &pkgmeta.Configuration{
					Spec: pkgmeta.ConfigurationSpec{
						MetaSpec: pkgmeta.MetaSpec{
							DependsOn: []pkgmeta.Dependency{
								{
									Provider: pointer.StringPtr("not-here-1"),
								},
								{
									Provider: pointer.StringPtr("not-here-2"),
								},
							},
						},
					},
				},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package:      "hasheddan/config-nop-a:v0.0.1",
						DesiredState: v1beta1.PackageRevisionActive,
					},
				},
			},
			want: want{
				total: 2,
				err:   errors.Errorf(errMissingDependenciesFmt, []string{"not-here-1", "not-here-2"}),
			},
		},
		"ErrorSelfExistMissingDependencies": {
			reason: "Should return error if self exists and missing dependencies.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
							l := obj.(*v1alpha1.Lock)
							l.Packages = []v1alpha1.LockPackage{
								{
									Source: "hasheddan/config-nop-a",
									Dependencies: []v1alpha1.Dependency{
										{
											Package: "not-here-1",
											Type:    v1alpha1.ProviderPackageType,
										},
										{
											Package: "not-here-2",
											Type:    v1alpha1.ConfigurationPackageType,
										},
									},
								},
								{
									Source: "not-here-1",
									Dependencies: []v1alpha1.Dependency{
										{
											Package: "not-here-3",
											Type:    v1alpha1.ProviderPackageType,
										},
									},
								},
							}
							return nil
						}),
						MockUpdate: test.NewMockUpdateFn(nil),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(nodes []dag.Node, fns ...dag.NodeFn) ([]dag.Node, error) {
								for i, n := range nodes {
									for _, f := range fns {
										f(i, n)
									}
								}
								return []dag.Node{
									&v1alpha1.Dependency{
										Package: "not-here-2",
									},
									&v1alpha1.Dependency{
										Package: "not-here-3",
									},
								}, nil
							},
							MockTraceNode: func(_ string) (map[string]dag.Node, error) {
								return map[string]dag.Node{
									"not-here-1": &v1alpha1.Dependency{},
									"not-here-2": &v1alpha1.Dependency{},
									"not-here-3": &v1alpha1.Dependency{},
								}, nil
							},
						}
					},
				},
				meta: &pkgmeta.Configuration{
					Spec: pkgmeta.ConfigurationSpec{
						MetaSpec: pkgmeta.MetaSpec{
							DependsOn: []pkgmeta.Dependency{
								{
									Provider: pointer.StringPtr("not-here-1"),
								},
								{
									Provider: pointer.StringPtr("not-here-2"),
								},
							},
						},
					},
				},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package:      "hasheddan/config-nop-a:v0.0.1",
						DesiredState: v1beta1.PackageRevisionActive,
					},
				},
			},
			want: want{
				total:     3,
				installed: 1,
				err:       errors.Errorf(errMissingDependenciesFmt, []string{"not-here-2", "not-here-3"}),
			},
		},
		"ErrorSelfExistInvalidDependencies": {
			reason: "Should return error if self exists and missing dependencies.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
							l := obj.(*v1alpha1.Lock)
							l.Packages = []v1alpha1.LockPackage{
								{
									Source: "hasheddan/config-nop-a",
									Dependencies: []v1alpha1.Dependency{
										{
											Package: "not-here-1",
											Type:    v1alpha1.ProviderPackageType,
										},
										{
											Package: "not-here-2",
											Type:    v1alpha1.ConfigurationPackageType,
										},
									},
								},
								{
									Source: "not-here-1",
									Dependencies: []v1alpha1.Dependency{
										{
											Package: "not-here-3",
											Type:    v1alpha1.ProviderPackageType,
										},
									},
								},
							}
							return nil
						}),
						MockUpdate: test.NewMockUpdateFn(nil),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(nodes []dag.Node, fns ...dag.NodeFn) ([]dag.Node, error) {
								for i, n := range nodes {
									for _, f := range fns {
										f(i, n)
									}
								}
								return nil, nil
							},
							MockTraceNode: func(_ string) (map[string]dag.Node, error) {
								return map[string]dag.Node{
									"not-here-1": &v1alpha1.Dependency{},
									"not-here-2": &v1alpha1.Dependency{},
									"not-here-3": &v1alpha1.Dependency{},
								}, nil
							},
							MockGetNode: func(s string) (dag.Node, error) {
								if s == "not-here-1" {
									return &v1alpha1.LockPackage{
										Source:  "not-here-1",
										Version: "v0.0.1",
									}, nil
								}
								if s == "not-here-2" {
									return &v1alpha1.LockPackage{
										Source:  "not-here-2",
										Version: "v0.0.1",
									}, nil
								}
								return nil, nil
							},
						}
					},
				},
				meta: &pkgmeta.Configuration{
					Spec: pkgmeta.ConfigurationSpec{
						MetaSpec: pkgmeta.MetaSpec{
							DependsOn: []pkgmeta.Dependency{
								{
									Provider: pointer.StringPtr("not-here-1"),
									Version:  ">=v0.1.0",
								},
								{
									Provider: pointer.StringPtr("not-here-2"),
									Version:  ">=v0.1.0",
								},
							},
						},
					},
				},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package:      "hasheddan/config-nop-a:v0.0.1",
						DesiredState: v1beta1.PackageRevisionActive,
					},
				},
			},
			want: want{
				total:     3,
				installed: 3,
				invalid:   2,
				err:       errors.Errorf(errIncompatibleDependencyFmt, []string{"not-here-1", "not-here-2"}),
			},
		},
		"SuccessfulSelfExistValidDependencies": {
			reason: "Should not return error if self exists, all dependencies exist and are valid.",
			args: args{
				dep: &PackageDependencyManager{
					client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
							l := obj.(*v1alpha1.Lock)
							l.Packages = []v1alpha1.LockPackage{
								{
									Source: "hasheddan/config-nop-a",
									Dependencies: []v1alpha1.Dependency{
										{
											Package: "not-here-1",
											Type:    v1alpha1.ProviderPackageType,
										},
										{
											Package: "not-here-2",
											Type:    v1alpha1.ConfigurationPackageType,
										},
									},
								},
								{
									Source: "not-here-1",
									Dependencies: []v1alpha1.Dependency{
										{
											Package: "not-here-3",
											Type:    v1alpha1.ProviderPackageType,
										},
									},
								},
							}
							return nil
						}),
						MockUpdate: test.NewMockUpdateFn(nil),
					},
					newDag: func() dag.DAG {
						return &dagfake.MockDag{
							MockInit: func(nodes []dag.Node, fns ...dag.NodeFn) ([]dag.Node, error) {
								for i, n := range nodes {
									for _, f := range fns {
										f(i, n)
									}
								}
								return nil, nil
							},
							MockTraceNode: func(_ string) (map[string]dag.Node, error) {
								return map[string]dag.Node{
									"not-here-1": &v1alpha1.Dependency{},
									"not-here-2": &v1alpha1.Dependency{},
									"not-here-3": &v1alpha1.Dependency{},
								}, nil
							},
							MockGetNode: func(s string) (dag.Node, error) {
								if s == "not-here-1" {
									return &v1alpha1.LockPackage{
										Source:  "not-here-1",
										Version: "v0.20.0",
									}, nil
								}
								if s == "not-here-2" {
									return &v1alpha1.LockPackage{
										Source:  "not-here-2",
										Version: "v0.100.1",
									}, nil
								}
								return nil, nil
							},
						}
					},
				},
				meta: &pkgmeta.Configuration{
					Spec: pkgmeta.ConfigurationSpec{
						MetaSpec: pkgmeta.MetaSpec{
							DependsOn: []pkgmeta.Dependency{
								{
									Provider: pointer.StringPtr("not-here-1"),
									Version:  ">=v0.1.0",
								},
								{
									Provider: pointer.StringPtr("not-here-2"),
									Version:  ">=v0.1.0",
								},
							},
						},
					},
				},
				pr: &v1beta1.ConfigurationRevision{
					Spec: v1beta1.PackageRevisionSpec{
						Package:      "hasheddan/config-nop-a:v0.0.1",
						DesiredState: v1beta1.PackageRevisionActive,
					},
				},
			},
			want: want{
				total:     3,
				installed: 3,
				invalid:   0,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			total, installed, invalid, err := tc.args.dep.Resolve(context.TODO(), tc.args.meta, tc.args.pr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\np.Resolve(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.total, total); diff != "" {
				t.Errorf("\n%s\nTotal(...): -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.installed, installed); diff != "" {
				t.Errorf("\n%s\nInstalled(...): -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.invalid, invalid); diff != "" {
				t.Errorf("\n%s\nInvalid(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
