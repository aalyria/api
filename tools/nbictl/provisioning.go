// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
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

package nbictl

import (
	"errors"
	"fmt"
	"os"
	"sort"

	provapipb "aalyria.com/spacetime/api/provisioning/v1alpha"
	provnbipb "aalyria.com/spacetime/tools/nbictl/provisioning"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/proto"
)

const provisioningAPISubDomain = "provisioning-v1alpha"

type ProvisioningResources struct {
	p2pSrTePolicies             map[string]*provapipb.P2PSrTePolicy
	p2pSrTePolicyCandidatePaths map[string]*provapipb.P2PSrTePolicyCandidatePath
	downtimes                   map[string]*provapipb.Downtime
	protectionAssociationGroups map[string]*provapipb.ProtectionAssociationGroup
	disjointAssociationGroups   map[string]*provapipb.DisjointAssociationGroup
}

func NewProvisioningResources() *ProvisioningResources {
	return &ProvisioningResources{
		p2pSrTePolicies:             map[string]*provapipb.P2PSrTePolicy{},
		p2pSrTePolicyCandidatePaths: map[string]*provapipb.P2PSrTePolicyCandidatePath{},
		downtimes:                   map[string]*provapipb.Downtime{},
		protectionAssociationGroups: map[string]*provapipb.ProtectionAssociationGroup{},
		disjointAssociationGroups:   map[string]*provapipb.DisjointAssociationGroup{},
	}
}

func (pr *ProvisioningResources) ResourceCount() int {
	return len(pr.p2pSrTePolicies) +
		len(pr.p2pSrTePolicyCandidatePaths) +
		len(pr.downtimes) +
		len(pr.protectionAssociationGroups) +
		len(pr.disjointAssociationGroups)
}

func (pr *ProvisioningResources) InsertProvisioningResources(resources *provnbipb.ProvisioningResources) {
	pr.insertP2PSrTePolicies(resources.GetP2PSrTePolicies())
	pr.insertP2PSrTePolicyCandidatePaths(resources.GetP2PSrTePolicyCandidatePaths())
	pr.insertDowntimes(resources.GetDowntimes())
	pr.insertProtectionAssociationGroups(resources.GetProtectionAssociationGroups())
	pr.insertDisjointAssociationGroups(resources.GetDisjointAssociationGroups())
}

func (pr *ProvisioningResources) ReadRemoteResources(appCtx *cli.Context, client provapipb.ProvisioningClient) error {
	p := pool.New().WithErrors()

	p.Go(func() error {
		result, err := client.ListP2PSrTePolicies(appCtx.Context, &provapipb.ListP2PSrTePoliciesRequest{})
		if err != nil {
			return err
		}
		pr.insertP2PSrTePolicies(result.GetP2PSrTePolicies())
		return nil
	})
	p.Go(func() error {
		keys := lo.Keys(pr.p2pSrTePolicies)
		sort.Strings(keys)
		for _, key := range keys {
			result, err := client.ListP2PSrTePolicyCandidatePaths(appCtx.Context, &provapipb.ListP2PSrTePolicyCandidatePathsRequest{
				Parent: key,
			})
			if err != nil {
				return err
			}
			pr.insertP2PSrTePolicyCandidatePaths(result.GetP2PSrTePolicyCandidatePaths())
		}
		return nil
	})
	p.Go(func() error {
		result, err := client.ListDowntimes(appCtx.Context, &provapipb.ListDowntimesRequest{})
		if err != nil {
			return err
		}
		pr.insertDowntimes(result.GetDowntimes())
		return nil
	})
	p.Go(func() error {
		result, err := client.ListProtectionAssociationGroups(appCtx.Context, &provapipb.ListProtectionAssociationGroupsRequest{})
		if err != nil {
			return err
		}
		pr.insertProtectionAssociationGroups(result.GetProtectionAssociationGroups())
		return nil
	})
	p.Go(func() error {
		result, err := client.ListDisjointAssociationGroups(appCtx.Context, &provapipb.ListDisjointAssociationGroupsRequest{})
		if err != nil {
			return err
		}
		pr.insertDisjointAssociationGroups(result.GetDisjointAssociationGroups())
		return nil
	})

	return p.Wait()
}

func (pr *ProvisioningResources) insertP2PSrTePolicies(entries []*provapipb.P2PSrTePolicy) {
	for _, entry := range entries {
		pr.p2pSrTePolicies[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertP2PSrTePolicyCandidatePaths(entries []*provapipb.P2PSrTePolicyCandidatePath) {
	for _, entry := range entries {
		pr.p2pSrTePolicyCandidatePaths[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertDowntimes(entries []*provapipb.Downtime) {
	for _, entry := range entries {
		pr.downtimes[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertProtectionAssociationGroups(entries []*provapipb.ProtectionAssociationGroup) {
	for _, entry := range entries {
		pr.protectionAssociationGroups[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertDisjointAssociationGroups(entries []*provapipb.DisjointAssociationGroup) {
	for _, entry := range entries {
		pr.disjointAssociationGroups[entry.GetName()] = entry
	}
}

func provisioningResourcesAreEquivalent[T proto.Message](a, b T) bool {
	// TODO: find a more robust equivalency check.
	return proto.Equal(a, b)
}

func countValues(m map[string][]string) int {
	count := 0

	for _, valueSlice := range m {
		count += len(valueSlice)
	}

	return count
}

func ProvisioningSync(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	if !appCtx.Args().Present() {
		return fmt.Errorf("sync needs at least one directory or filename argument")
	}

	// Step 1: load up all local provisioning resources.
	localResources := NewProvisioningResources()

	localFiles, err := findAllFilesWithExtension(marshaller.fileExt, appCtx.Bool("recursive"), appCtx.Args().Slice()...)
	if err != nil {
		return err
	}
	if len(localFiles) == 0 {
		return fmt.Errorf("found no local files to read")
	}
	for _, localFile := range localFiles {
		fmt.Fprintf(os.Stdout, "reading %s\n", localFile)
		contents, err := os.ReadFile(localFile)
		if err != nil {
			return err
		}

		resources := &provnbipb.ProvisioningResources{}
		err = marshaller.unmarshal(contents, resources)
		if err != nil {
			return err
		}
		localResources.InsertProvisioningResources(resources)
	}
	if localResources.ResourceCount() == 0 {
		return fmt.Errorf("found no local resources to sync")
	}
	fmt.Fprintf(os.Stdout, "\nfound local resources:\n")
	fmt.Fprintf(os.Stdout, "- %d P2PSrTePolicies\n", len(localResources.p2pSrTePolicies))
	fmt.Fprintf(os.Stdout, "- %d P2PSrTePolicyCandidatePaths\n", len(localResources.p2pSrTePolicyCandidatePaths))
	fmt.Fprintf(os.Stdout, "- %d Downtimes\n", len(localResources.downtimes))
	fmt.Fprintf(os.Stdout, "- %d ProtectionAssociationGroups\n", len(localResources.protectionAssociationGroups))
	fmt.Fprintf(os.Stdout, "- %d DisjointAssociationGroups\n", len(localResources.disjointAssociationGroups))

	// Step 2: load up all the remote instance resources.
	remoteResources := NewProvisioningResources()

	conn, err := openAPIConnection(appCtx, provisioningAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	provisioningClient := provapipb.NewProvisioningClient(conn)
	if err := remoteResources.ReadRemoteResources(appCtx, provisioningClient); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "\nfound remote resources:\n")
	fmt.Fprintf(os.Stdout, "- %d P2PSrTePolicies\n", len(remoteResources.p2pSrTePolicies))
	fmt.Fprintf(os.Stdout, "- %d P2PSrTePolicyCandidatePaths\n", len(remoteResources.p2pSrTePolicyCandidatePaths))
	fmt.Fprintf(os.Stdout, "- %d Downtimes\n", len(remoteResources.downtimes))
	fmt.Fprintf(os.Stdout, "- %d ProtectionAssociationGroups\n", len(remoteResources.protectionAssociationGroups))
	fmt.Fprintf(os.Stdout, "- %d DisjointAssociationGroups\n", len(remoteResources.disjointAssociationGroups))

	// Step 3: compute and print/enact differences.
	deleteMode := appCtx.Bool("delete")
	dryRunMode := appCtx.Bool("dry-run")
	verboseMode := appCtx.Bool("verbose")
	printMode := dryRunMode || verboseMode

	// TODO: restructure these computations for scalability.
	resourcesToBeAdded := map[string][]string{}
	resourcesToBeAdded["p2pSrTePolicies"] = lo.Without(lo.Keys(localResources.p2pSrTePolicies), lo.Keys(remoteResources.p2pSrTePolicies)...)
	resourcesToBeAdded["p2pSrTePolicyCandidatePaths"] = lo.Without(lo.Keys(localResources.p2pSrTePolicyCandidatePaths), lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths)...)
	resourcesToBeAdded["downtimes"] = lo.Without(lo.Keys(localResources.downtimes), lo.Keys(remoteResources.downtimes)...)
	resourcesToBeAdded["protectionAssociationGroups"] = lo.Without(lo.Keys(localResources.protectionAssociationGroups), lo.Keys(remoteResources.protectionAssociationGroups)...)
	resourcesToBeAdded["disjointAssociationGroups"] = lo.Without(lo.Keys(localResources.disjointAssociationGroups), lo.Keys(remoteResources.disjointAssociationGroups)...)

	resourcesInCommon := map[string][]string{}
	resourcesInCommon["p2pSrTePolicies"] = lo.Intersect(lo.Keys(localResources.p2pSrTePolicies), lo.Keys(remoteResources.p2pSrTePolicies))
	resourcesInCommon["p2pSrTePolicyCandidatePaths"] = lo.Intersect(lo.Keys(localResources.p2pSrTePolicyCandidatePaths), lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths))
	resourcesInCommon["downtimes"] = lo.Intersect(lo.Keys(localResources.downtimes), lo.Keys(remoteResources.downtimes))
	resourcesInCommon["protectionAssociationGroups"] = lo.Intersect(lo.Keys(localResources.protectionAssociationGroups), lo.Keys(remoteResources.protectionAssociationGroups))
	resourcesInCommon["disjointAssociationGroups"] = lo.Intersect(lo.Keys(localResources.disjointAssociationGroups), lo.Keys(remoteResources.disjointAssociationGroups))

	fmt.Fprintf(os.Stdout, "\ncomparing local and remote resources:\n")
	fmt.Fprintf(os.Stdout, "- %d resources to be added\n", countValues(resourcesToBeAdded))
	fmt.Fprintf(os.Stdout, "- %d resources to be evaluated for update\n", countValues(resourcesInCommon))

	errs := []error{}

	///
	// Maybe delete resources.
	if deleteMode {
		resourcesToBeDeleted := map[string][]string{}
		resourcesToBeDeleted["p2pSrTePolicies"] = lo.Without(lo.Keys(remoteResources.p2pSrTePolicies), lo.Keys(localResources.p2pSrTePolicies)...)
		resourcesToBeDeleted["p2pSrTePolicyCandidatePaths"] = lo.Without(lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths), lo.Keys(localResources.p2pSrTePolicyCandidatePaths)...)
		resourcesToBeDeleted["downtimes"] = lo.Without(lo.Keys(remoteResources.downtimes), lo.Keys(localResources.downtimes)...)
		resourcesToBeDeleted["protectionAssociationGroups"] = lo.Without(lo.Keys(remoteResources.protectionAssociationGroups), lo.Keys(localResources.protectionAssociationGroups)...)
		resourcesToBeDeleted["disjointAssociationGroups"] = lo.Without(lo.Keys(remoteResources.disjointAssociationGroups), lo.Keys(localResources.disjointAssociationGroups)...)

		fmt.Fprintf(os.Stdout, "- %d resources to be deleted\n", countValues(resourcesToBeDeleted))

		p := pool.New().WithErrors()

		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				resourcesToBeDeleted["p2pSrTePolicies"], printMode, dryRunMode, func(policy string) error {
					_, err := provisioningClient.DeleteP2PSrTePolicy(appCtx.Context, &provapipb.DeleteP2PSrTePolicyRequest{
						Name: policy,
					})
					return err
				})...)
		})
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				resourcesToBeDeleted["p2pSrTePolicyCandidatePath"], printMode, dryRunMode, func(path string) error {
					_, err := provisioningClient.DeleteP2PSrTePolicyCandidatePath(appCtx.Context, &provapipb.DeleteP2PSrTePolicyCandidatePathRequest{
						Name: path,
					})
					return err
				})...)
		})
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				resourcesToBeDeleted["downtimes"], printMode, dryRunMode, func(downtime string) error {
					_, err := provisioningClient.DeleteDowntime(appCtx.Context, &provapipb.DeleteDowntimeRequest{
						Name: downtime,
					})
					return err
				})...)
		})
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				resourcesToBeDeleted["protectionAssociationGroups"], printMode, dryRunMode, func(protectionAssociationGroup string) error {
					_, err := provisioningClient.DeleteProtectionAssociationGroup(appCtx.Context, &provapipb.DeleteProtectionAssociationGroupRequest{
						Name: protectionAssociationGroup,
					})
					return err
				})...)
		})
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				resourcesToBeDeleted["disjointAssociationGroups"], printMode, dryRunMode, func(disjointAssociationGroup string) error {
					_, err := provisioningClient.DeleteDisjointAssociationGroup(appCtx.Context, &provapipb.DeleteDisjointAssociationGroupRequest{
						Name: disjointAssociationGroup,
					})
					return err
				})...)
		})

		errs = append(errs, p.Wait())
	}

	p := pool.New().WithErrors()

	///
	// Update resources.
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.P2PSrTePolicy](
			resourcesInCommon["p2pSrTePolicies"], localResources.p2pSrTePolicies, remoteResources.p2pSrTePolicies, printMode, dryRunMode, func(policy *provapipb.P2PSrTePolicy) error {
				_, err := provisioningClient.UpdateP2PSrTePolicy(appCtx.Context, &provapipb.UpdateP2PSrTePolicyRequest{
					Policy:       policy,
					AllowMissing: false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.P2PSrTePolicyCandidatePath](
			resourcesInCommon["p2pSrTePolicyCandidatePaths"], localResources.p2pSrTePolicyCandidatePaths, remoteResources.p2pSrTePolicyCandidatePaths, printMode, dryRunMode, func(path *provapipb.P2PSrTePolicyCandidatePath) error {
				_, err := provisioningClient.UpdateP2PSrTePolicyCandidatePath(appCtx.Context, &provapipb.UpdateP2PSrTePolicyCandidatePathRequest{
					Path:         path,
					AllowMissing: false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.Downtime](
			resourcesInCommon["downtimes"], localResources.downtimes, remoteResources.downtimes, printMode, dryRunMode, func(downtime *provapipb.Downtime) error {
				_, err := provisioningClient.UpdateDowntime(appCtx.Context, &provapipb.UpdateDowntimeRequest{
					Downtime:     downtime,
					AllowMissing: false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.ProtectionAssociationGroup](
			resourcesInCommon["protectionAssociationGroups"], localResources.protectionAssociationGroups, remoteResources.protectionAssociationGroups, printMode, dryRunMode, func(protectionAssociationGroup *provapipb.ProtectionAssociationGroup) error {
				_, err := provisioningClient.UpdateProtectionAssociationGroup(appCtx.Context, &provapipb.UpdateProtectionAssociationGroupRequest{
					ProtectionAssociationGroup: protectionAssociationGroup,
					AllowMissing:               false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.DisjointAssociationGroup](
			resourcesInCommon["disjointAssociationGroups"], localResources.disjointAssociationGroups, remoteResources.disjointAssociationGroups, printMode, dryRunMode, func(disjointAssociationGroup *provapipb.DisjointAssociationGroup) error {
				_, err := provisioningClient.UpdateDisjointAssociationGroup(appCtx.Context, &provapipb.UpdateDisjointAssociationGroupRequest{
					DisjointAssociationGroup: disjointAssociationGroup,
					AllowMissing:             false,
				})
				return err
			})...)
	})

	///
	// Add resources.
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.P2PSrTePolicy](
			resourcesToBeAdded["p2pSrTePolicies"], localResources.p2pSrTePolicies, printMode, dryRunMode, func(policy *provapipb.P2PSrTePolicy) error {
				_, err := provisioningClient.UpdateP2PSrTePolicy(appCtx.Context, &provapipb.UpdateP2PSrTePolicyRequest{
					Policy:       policy,
					AllowMissing: true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.P2PSrTePolicyCandidatePath](
			resourcesToBeAdded["p2pSrTePolicyCandidatePaths"], localResources.p2pSrTePolicyCandidatePaths, printMode, dryRunMode, func(path *provapipb.P2PSrTePolicyCandidatePath) error {
				_, err := provisioningClient.UpdateP2PSrTePolicyCandidatePath(appCtx.Context, &provapipb.UpdateP2PSrTePolicyCandidatePathRequest{
					Path:         path,
					AllowMissing: true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.Downtime](
			resourcesToBeAdded["downtimes"], localResources.downtimes, printMode, dryRunMode, func(downtime *provapipb.Downtime) error {
				_, err := provisioningClient.UpdateDowntime(appCtx.Context, &provapipb.UpdateDowntimeRequest{
					Downtime:     downtime,
					AllowMissing: true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.ProtectionAssociationGroup](
			resourcesToBeAdded["protectionAssociationGroups"], localResources.protectionAssociationGroups, printMode, dryRunMode, func(protectionAssociationGroup *provapipb.ProtectionAssociationGroup) error {
				_, err := provisioningClient.UpdateProtectionAssociationGroup(appCtx.Context, &provapipb.UpdateProtectionAssociationGroupRequest{
					ProtectionAssociationGroup: protectionAssociationGroup,
					AllowMissing:               true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.DisjointAssociationGroup](
			resourcesToBeAdded["disjointAssociationGroups"], localResources.disjointAssociationGroups, printMode, dryRunMode, func(disjointAssociationGroup *provapipb.DisjointAssociationGroup) error {
				_, err := provisioningClient.UpdateDisjointAssociationGroup(appCtx.Context, &provapipb.UpdateDisjointAssociationGroupRequest{
					DisjointAssociationGroup: disjointAssociationGroup,
					AllowMissing:             true,
				})
				return err
			})...)
	})

	errs = append(errs, p.Wait())
	return errors.Join(errs...)
}

func createRemoteResources[T proto.Message](resourceIDs []string, localResources map[string]T, printMode bool, dryRunMode bool, createFn func(T) error) []error {
	errs := []error{}

	sort.Strings(resourceIDs)
	for _, key := range resourceIDs {
		if printMode {
			fmt.Printf("add resource: %s\n", key)
		}
		if !dryRunMode {
			if err := createFn(localResources[key]); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

func updateRemoteResources[T proto.Message](
	resourceIDs []string,
	localResources map[string]T, remoteResources map[string]T,
	printMode bool, dryRunMode bool,
	updateFn func(T) error) []error {
	errs := []error{}

	sort.Strings(resourceIDs)
	for _, key := range resourceIDs {
		if provisioningResourcesAreEquivalent(localResources[key], remoteResources[key]) {
			continue
		}
		if printMode {
			fmt.Printf("update resource: %s\n", key)
		}
		if !dryRunMode {
			if err := updateFn(localResources[key]); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

func deleteRemoteResources(resourceIDs []string, printMode bool, dryRunMode bool, deleteFn func(string) error) []error {
	errs := []error{}

	sort.Strings(resourceIDs)
	for _, key := range resourceIDs {
		if printMode {
			fmt.Printf("delete entity: %s\n", key)
		}
		if !dryRunMode {
			if err := deleteFn(key); err != nil && !isNotFoundError(err) {
				errs = append(errs, err)
			}
		}
	}

	return errs
}
