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
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/proto"

	provapipb "aalyria.com/spacetime/api/provisioning/v1alpha"
	provnbipb "aalyria.com/spacetime/tools/nbictl/provisioning"
)

const provisioningAPISubDomain = "provisioning-v1alpha"

type ProvisioningResources struct {
	p2pSrTePolicies              map[string]*provapipb.P2PSrTePolicy
	p2pSrTePolicyCandidatePaths  map[string]*provapipb.P2PSrTePolicyCandidatePath
	p2mpSrTePolicies             map[string]*provapipb.P2MpSrTePolicy
	p2mpSrTePolicyCandidatePaths map[string]*provapipb.P2MpSrTePolicyCandidatePath
	downtimes                    map[string]*provapipb.Downtime
	protectionAssociationGroups  map[string]*provapipb.ProtectionAssociationGroup
	disjointAssociationGroups    map[string]*provapipb.DisjointAssociationGroup
	links                        map[string]*provapipb.Link
	geographicRegions            map[string]*provapipb.GeographicRegion
	emissionsLimits              map[string]*provapipb.EmissionsLimit
	emissionsTargets             map[string]*provapipb.EmissionsTarget
}

func (pr *ProvisioningResources) String() string {
	keys := slices.Concat(
		lo.Keys(pr.p2pSrTePolicies),
		lo.Keys(pr.p2pSrTePolicyCandidatePaths),
		lo.Keys(pr.p2mpSrTePolicies),
		lo.Keys(pr.p2mpSrTePolicyCandidatePaths),
		lo.Keys(pr.downtimes),
		lo.Keys(pr.protectionAssociationGroups),
		lo.Keys(pr.disjointAssociationGroups),
		lo.Keys(pr.links),
		lo.Keys(pr.geographicRegions),
		lo.Keys(pr.emissionsLimits),
		lo.Keys(pr.emissionsTargets),
	)

	if len(keys) == 0 {
		return ""
	}

	slices.Sort(keys)

	return "- " + strings.Join(keys, "\n- ")
}

func marshalMap[T proto.Message](m map[string]T, marshaller protoFormat) map[string]string {
	return lo.MapValues(m, func(value T, _ string) string {
		return string(lo.Must(marshaller.marshal(value)))
	})
}

func (pr *ProvisioningResources) MarshalledString(marshaller protoFormat) string {
	entries := slices.Concat(
		lo.Entries(marshalMap(pr.p2pSrTePolicies, marshaller)),
		lo.Entries(marshalMap(pr.p2pSrTePolicyCandidatePaths, marshaller)),
		lo.Entries(marshalMap(pr.p2mpSrTePolicies, marshaller)),
		lo.Entries(marshalMap(pr.p2mpSrTePolicyCandidatePaths, marshaller)),
		lo.Entries(marshalMap(pr.downtimes, marshaller)),
		lo.Entries(marshalMap(pr.protectionAssociationGroups, marshaller)),
		lo.Entries(marshalMap(pr.disjointAssociationGroups, marshaller)),
		lo.Entries(marshalMap(pr.links, marshaller)),
		lo.Entries(marshalMap(pr.geographicRegions, marshaller)),
		lo.Entries(marshalMap(pr.emissionsLimits, marshaller)),
		lo.Entries(marshalMap(pr.emissionsTargets, marshaller)),
	)
	slices.SortFunc(entries, func(e1, e2 lo.Entry[string, string]) int { return strings.Compare(e1.Key, e2.Key) })

	sortedValues := lo.Map(entries, func(item lo.Entry[string, string], index int) string {
		return item.Value
	})
	return strings.Join(sortedValues, "\n")
}

func NewProvisioningResources() *ProvisioningResources {
	return &ProvisioningResources{
		p2pSrTePolicies:              map[string]*provapipb.P2PSrTePolicy{},
		p2pSrTePolicyCandidatePaths:  map[string]*provapipb.P2PSrTePolicyCandidatePath{},
		p2mpSrTePolicies:             map[string]*provapipb.P2MpSrTePolicy{},
		p2mpSrTePolicyCandidatePaths: map[string]*provapipb.P2MpSrTePolicyCandidatePath{},
		downtimes:                    map[string]*provapipb.Downtime{},
		protectionAssociationGroups:  map[string]*provapipb.ProtectionAssociationGroup{},
		disjointAssociationGroups:    map[string]*provapipb.DisjointAssociationGroup{},
		links:                        map[string]*provapipb.Link{},
		geographicRegions:            map[string]*provapipb.GeographicRegion{},
		emissionsLimits:              map[string]*provapipb.EmissionsLimit{},
		emissionsTargets:             map[string]*provapipb.EmissionsTarget{},
	}
}

func (pr *ProvisioningResources) ResourceCount() int {
	return len(pr.p2pSrTePolicies) +
		len(pr.p2pSrTePolicyCandidatePaths) +
		len(pr.p2mpSrTePolicies) +
		len(pr.p2mpSrTePolicyCandidatePaths) +
		len(pr.downtimes) +
		len(pr.protectionAssociationGroups) +
		len(pr.disjointAssociationGroups) +
		len(pr.links) +
		len(pr.geographicRegions) +
		len(pr.emissionsLimits) +
		len(pr.emissionsTargets)
}

func (pr *ProvisioningResources) InsertProvisioningResources(resources *provnbipb.ProvisioningResources) {
	pr.insertP2PSrTePolicies(resources.GetP2PSrTePolicies())
	pr.insertP2PSrTePolicyCandidatePaths(resources.GetP2PSrTePolicyCandidatePaths())
	pr.insertP2MpSrTePolicies(resources.GetP2MpSrTePolicies())
	pr.insertP2MpSrTePolicyCandidatePaths(resources.GetP2MpSrTePolicyCandidatePaths())
	pr.insertDowntimes(resources.GetDowntimes())
	pr.insertProtectionAssociationGroups(resources.GetProtectionAssociationGroups())
	pr.insertDisjointAssociationGroups(resources.GetDisjointAssociationGroups())
	pr.insertLinks(resources.GetLinks())
	pr.insertGeographicRegions(resources.GetGeographicRegions())
	pr.insertEmissionsLimits(resources.GetEmissionsLimits())
	pr.insertEmissionsTargets(resources.GetEmissionsTargets())
}

func ProvisioningResourcesFromRemote(ctx context.Context, client provapipb.ProvisioningClient) (*ProvisioningResources, error) {
	pr := NewProvisioningResources()

	p := pool.New().WithErrors()

	var downtimes []*provapipb.Downtime
	p.Go(func() error {
		result, err := client.ListDowntimes(ctx, &provapipb.ListDowntimesRequest{})
		if err != nil {
			return err
		}
		downtimes = result.GetDowntimes()
		return nil
	})

	var protectionAssociationGroups []*provapipb.ProtectionAssociationGroup
	p.Go(func() error {
		result, err := client.ListProtectionAssociationGroups(ctx, &provapipb.ListProtectionAssociationGroupsRequest{})
		if err != nil {
			return err
		}
		protectionAssociationGroups = result.GetProtectionAssociationGroups()
		return nil
	})

	var disjointAssociationGroups []*provapipb.DisjointAssociationGroup
	p.Go(func() error {
		result, err := client.ListDisjointAssociationGroups(ctx, &provapipb.ListDisjointAssociationGroupsRequest{})
		if err != nil {
			return err
		}
		disjointAssociationGroups = result.GetDisjointAssociationGroups()
		return nil
	})

	var links []*provapipb.Link
	p.Go(func() error {
		result, err := client.ListLinks(ctx, &provapipb.ListLinksRequest{})
		if err != nil {
			return err
		}
		links = result.GetLinks()
		return nil
	})

	var geographicRegions []*provapipb.GeographicRegion
	p.Go(func() error {
		result, err := client.ListGeographicRegions(ctx, &provapipb.ListGeographicRegionsRequest{})
		if err != nil {
			return err
		}
		geographicRegions = result.GetGeographicRegions()
		return nil
	})

	var emissionsLimits []*provapipb.EmissionsLimit
	p.Go(func() error {
		result, err := client.ListEmissionsLimits(ctx, &provapipb.ListEmissionsLimitsRequest{})
		if err != nil {
			return err
		}
		emissionsLimits = result.GetEmissionsLimits()
		return nil
	})

	var emissionsTargets []*provapipb.EmissionsTarget
	p.Go(func() error {
		result, err := client.ListEmissionsTargets(ctx, &provapipb.ListEmissionsTargetsRequest{})
		if err != nil {
			return err
		}
		emissionsTargets = result.GetEmissionsTargets()
		return nil
	})

	err := p.Wait()
	if err != nil {
		return nil, err
	}

	pr.insertDowntimes(downtimes)
	pr.insertProtectionAssociationGroups(protectionAssociationGroups)
	pr.insertDisjointAssociationGroups(disjointAssociationGroups)
	pr.insertLinks(links)
	pr.insertGeographicRegions(geographicRegions)
	pr.insertEmissionsLimits(emissionsLimits)
	pr.insertEmissionsTargets(emissionsTargets)

	{
		result, err := client.ListP2PSrTePolicies(ctx, &provapipb.ListP2PSrTePoliciesRequest{})
		if err != nil {
			return nil, err
		}
		p2PSrTePolicies := result.GetP2PSrTePolicies()
		pr.insertP2PSrTePolicies(p2PSrTePolicies)

		keys := lo.Keys(pr.p2pSrTePolicies)
		sort.Strings(keys)

		candidatePathResults := make([][]*provapipb.P2PSrTePolicyCandidatePath, len(keys))
		p := pool.New().WithErrors()
		for i, key := range keys {
			p.Go(func() error {
				result, err := client.ListP2PSrTePolicyCandidatePaths(ctx, &provapipb.ListP2PSrTePolicyCandidatePathsRequest{
					Parent: key,
				})
				if err != nil {
					return err
				}
				candidatePathResults[i] = result.GetP2PSrTePolicyCandidatePaths()
				return nil
			})
		}
		err = p.Wait()
		if err != nil {
			return nil, err
		}
		pr.insertP2PSrTePolicyCandidatePaths(slices.Concat(candidatePathResults...))
	}

	{
		result, err := client.ListP2MpSrTePolicies(ctx, &provapipb.ListP2MpSrTePoliciesRequest{})
		if err != nil {
			return nil, err
		}
		p2mpSrTePolicies := result.GetP2MpSrTePolicies()
		pr.insertP2MpSrTePolicies(p2mpSrTePolicies)

		keys := lo.Keys(pr.p2mpSrTePolicies)
		sort.Strings(keys)

		candidatePathResults := make([][]*provapipb.P2MpSrTePolicyCandidatePath, len(keys))
		p := pool.New().WithErrors()
		for i, key := range keys {
			p.Go(func() error {
				result, err := client.ListP2MpSrTePolicyCandidatePaths(ctx, &provapipb.ListP2MpSrTePolicyCandidatePathsRequest{
					Parent: key,
				})
				if err != nil {
					return err
				}
				candidatePathResults[i] = result.GetP2MpSrTePolicyCandidatePaths()
				return nil
			})
		}
		err = p.Wait()
		if err != nil {
			return nil, err
		}
		pr.insertP2MpSrTePolicyCandidatePaths(slices.Concat(candidatePathResults...))
	}

	return pr, nil
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

func (pr *ProvisioningResources) insertP2MpSrTePolicies(entries []*provapipb.P2MpSrTePolicy) {
	for _, entry := range entries {
		pr.p2mpSrTePolicies[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertP2MpSrTePolicyCandidatePaths(entries []*provapipb.P2MpSrTePolicyCandidatePath) {
	for _, entry := range entries {
		pr.p2mpSrTePolicyCandidatePaths[entry.GetName()] = entry
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

func (pr *ProvisioningResources) insertLinks(entries []*provapipb.Link) {
	for _, entry := range entries {
		pr.links[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertGeographicRegions(entries []*provapipb.GeographicRegion) {
	for _, entry := range entries {
		pr.geographicRegions[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertEmissionsLimits(entries []*provapipb.EmissionsLimit) {
	for _, entry := range entries {
		pr.emissionsLimits[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertEmissionsTargets(entries []*provapipb.EmissionsTarget) {
	for _, entry := range entries {
		pr.emissionsTargets[entry.GetName()] = entry
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
	ctx := appCtx.Context
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
		fmt.Printf("reading %s\n", localFile)
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
	fmt.Printf("\nfound local resources:\n%s", localResources)

	// Step 2: load up all the remote instance resources.
	conn, err := openAPIConnection(appCtx, provisioningAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	provisioningClient := provapipb.NewProvisioningClient(conn)
	remoteResources, err := ProvisioningResourcesFromRemote(ctx, provisioningClient)
	if err != nil {
		return err
	}
	fmt.Printf("\nfound remote resources:\n%s", remoteResources)

	// Step 3: compute and print/enact differences.
	deleteMode := appCtx.Bool("delete")
	dryRunMode := appCtx.Bool("dry-run")
	verboseMode := appCtx.Bool("verbose")
	maxConcurrency := appCtx.Int("max-concurrency")
	printMode := dryRunMode || verboseMode

	// TODO: restructure these computations for scalability.
	resourcesToBeAdded := map[string][]string{
		"p2pSrTePolicies":             lo.Without(lo.Keys(localResources.p2pSrTePolicies), lo.Keys(remoteResources.p2pSrTePolicies)...),
		"p2pSrTePolicyCandidatePaths": lo.Without(lo.Keys(localResources.p2pSrTePolicyCandidatePaths), lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths)...),
		"downtimes":                   lo.Without(lo.Keys(localResources.downtimes), lo.Keys(remoteResources.downtimes)...),
		"protectionAssociationGroups": lo.Without(lo.Keys(localResources.protectionAssociationGroups), lo.Keys(remoteResources.protectionAssociationGroups)...),
		"disjointAssociationGroups":   lo.Without(lo.Keys(localResources.disjointAssociationGroups), lo.Keys(remoteResources.disjointAssociationGroups)...),
		"links":                       lo.Without(lo.Keys(localResources.links), lo.Keys(remoteResources.links)...),
		"geographicRegions":           lo.Without(lo.Keys(localResources.geographicRegions), lo.Keys(remoteResources.geographicRegions)...),
		"emissionsLimits":             lo.Without(lo.Keys(localResources.emissionsLimits), lo.Keys(remoteResources.emissionsLimits)...),
		"emissionsTargets":            lo.Without(lo.Keys(localResources.emissionsTargets), lo.Keys(remoteResources.emissionsTargets)...),
	}

	resourcesInCommon := map[string][]string{
		"p2pSrTePolicies":             lo.Intersect(lo.Keys(localResources.p2pSrTePolicies), lo.Keys(remoteResources.p2pSrTePolicies)),
		"p2pSrTePolicyCandidatePaths": lo.Intersect(lo.Keys(localResources.p2pSrTePolicyCandidatePaths), lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths)),
		"downtimes":                   lo.Intersect(lo.Keys(localResources.downtimes), lo.Keys(remoteResources.downtimes)),
		"protectionAssociationGroups": lo.Intersect(lo.Keys(localResources.protectionAssociationGroups), lo.Keys(remoteResources.protectionAssociationGroups)),
		"disjointAssociationGroups":   lo.Intersect(lo.Keys(localResources.disjointAssociationGroups), lo.Keys(remoteResources.disjointAssociationGroups)),
		"links":                       lo.Intersect(lo.Keys(localResources.links), lo.Keys(remoteResources.links)),
		"geographicRegions":           lo.Intersect(lo.Keys(localResources.geographicRegions), lo.Keys(remoteResources.geographicRegions)),
		"emissionsLimits":             lo.Intersect(lo.Keys(localResources.emissionsLimits), lo.Keys(remoteResources.emissionsLimits)),
		"emissionsTargets":            lo.Intersect(lo.Keys(localResources.emissionsTargets), lo.Keys(remoteResources.emissionsTargets)),
	}

	fmt.Println("\ncomparing local and remote resources:")
	fmt.Printf("- %d resources to be added\n", countValues(resourcesToBeAdded))
	fmt.Printf("- %d resources to be evaluated for update\n", countValues(resourcesInCommon))

	errs := []error{}

	///
	// Maybe delete resources.
	if deleteMode {
		deleteParams := deleteProvisioningParams{
			p2pSrTePolicies:             lo.Without(lo.Keys(remoteResources.p2pSrTePolicies), lo.Keys(localResources.p2pSrTePolicies)...),
			p2pSrTePolicyCandidatePaths: lo.Without(lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths), lo.Keys(localResources.p2pSrTePolicyCandidatePaths)...),
			downtimes:                   lo.Without(lo.Keys(remoteResources.downtimes), lo.Keys(localResources.downtimes)...),
			protectionAssociationGroups: lo.Without(lo.Keys(remoteResources.protectionAssociationGroups), lo.Keys(localResources.protectionAssociationGroups)...),
			disjointAssociationGroups:   lo.Without(lo.Keys(remoteResources.disjointAssociationGroups), lo.Keys(localResources.disjointAssociationGroups)...),
			links:                       lo.Without(lo.Keys(remoteResources.links), lo.Keys(localResources.links)...),
			geographicRegions:           lo.Without(lo.Keys(remoteResources.geographicRegions), lo.Keys(localResources.geographicRegions)...),
			emissionsLimits:             lo.Without(lo.Keys(remoteResources.emissionsLimits), lo.Keys(localResources.emissionsLimits)...),
			emissionsTargets:            lo.Without(lo.Keys(remoteResources.emissionsTargets), lo.Keys(localResources.emissionsTargets)...),

			printMode:  printMode,
			dryRunMode: dryRunMode,
			client:     provisioningClient,
		}

		fmt.Printf("- %d resources to be deleted\n", len(slices.Concat(
			deleteParams.p2pSrTePolicies,
			deleteParams.p2pSrTePolicyCandidatePaths,
			deleteParams.downtimes,
			deleteParams.protectionAssociationGroups,
			deleteParams.disjointAssociationGroups,
			deleteParams.links,
			deleteParams.geographicRegions,
			deleteParams.emissionsLimits,
			deleteParams.emissionsTargets,
		)))

		err := deleteProvisioning(ctx, deleteParams)
		errs = append(errs, err)
	}

	p := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)

	///
	// Update resources.
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.P2PSrTePolicy](
			resourcesInCommon["p2pSrTePolicies"], localResources.p2pSrTePolicies, remoteResources.p2pSrTePolicies, printMode, dryRunMode, func(policy *provapipb.P2PSrTePolicy) error {
				_, err := provisioningClient.UpdateP2PSrTePolicy(ctx, &provapipb.UpdateP2PSrTePolicyRequest{
					Policy:       policy,
					AllowMissing: false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.P2PSrTePolicyCandidatePath](
			resourcesInCommon["p2pSrTePolicyCandidatePaths"], localResources.p2pSrTePolicyCandidatePaths, remoteResources.p2pSrTePolicyCandidatePaths, printMode, dryRunMode, func(path *provapipb.P2PSrTePolicyCandidatePath) error {
				_, err := provisioningClient.UpdateP2PSrTePolicyCandidatePath(ctx, &provapipb.UpdateP2PSrTePolicyCandidatePathRequest{
					Path:         path,
					AllowMissing: false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.Downtime](
			resourcesInCommon["downtimes"], localResources.downtimes, remoteResources.downtimes, printMode, dryRunMode, func(downtime *provapipb.Downtime) error {
				_, err := provisioningClient.UpdateDowntime(ctx, &provapipb.UpdateDowntimeRequest{
					Downtime:     downtime,
					AllowMissing: false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.ProtectionAssociationGroup](
			resourcesInCommon["protectionAssociationGroups"], localResources.protectionAssociationGroups, remoteResources.protectionAssociationGroups, printMode, dryRunMode, func(protectionAssociationGroup *provapipb.ProtectionAssociationGroup) error {
				_, err := provisioningClient.UpdateProtectionAssociationGroup(ctx, &provapipb.UpdateProtectionAssociationGroupRequest{
					ProtectionAssociationGroup: protectionAssociationGroup,
					AllowMissing:               false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.DisjointAssociationGroup](
			resourcesInCommon["disjointAssociationGroups"], localResources.disjointAssociationGroups, remoteResources.disjointAssociationGroups, printMode, dryRunMode, func(disjointAssociationGroup *provapipb.DisjointAssociationGroup) error {
				_, err := provisioningClient.UpdateDisjointAssociationGroup(ctx, &provapipb.UpdateDisjointAssociationGroupRequest{
					DisjointAssociationGroup: disjointAssociationGroup,
					AllowMissing:             false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.Link](
			resourcesInCommon["links"], localResources.links, remoteResources.links, printMode, dryRunMode, func(link *provapipb.Link) error {
				_, err := provisioningClient.UpdateLink(ctx, &provapipb.UpdateLinkRequest{
					Link:         link,
					AllowMissing: false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.GeographicRegion](
			resourcesInCommon["geographicRegions"], localResources.geographicRegions, remoteResources.geographicRegions, printMode, dryRunMode, func(geographicRegion *provapipb.GeographicRegion) error {
				_, err := provisioningClient.UpdateGeographicRegion(ctx, &provapipb.UpdateGeographicRegionRequest{
					GeographicRegion: geographicRegion,
					AllowMissing:     false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.EmissionsLimit](
			resourcesInCommon["emissionsLimits"], localResources.emissionsLimits, remoteResources.emissionsLimits, printMode, dryRunMode, func(emissionsLimit *provapipb.EmissionsLimit) error {
				_, err := provisioningClient.UpdateEmissionsLimit(ctx, &provapipb.UpdateEmissionsLimitRequest{
					EmissionsLimit: emissionsLimit,
					AllowMissing:   false,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(updateRemoteResources[*provapipb.EmissionsTarget](
			resourcesInCommon["emissionsTargets"], localResources.emissionsTargets, remoteResources.emissionsTargets, printMode, dryRunMode, func(emissionsTarget *provapipb.EmissionsTarget) error {
				_, err := provisioningClient.UpdateEmissionsTarget(ctx, &provapipb.UpdateEmissionsTargetRequest{
					EmissionsTarget: emissionsTarget,
					AllowMissing:    false,
				})
				return err
			})...)
	})

	///
	// Add resources.
	p.Go(func() error {
		err := errors.Join(createRemoteResources[*provapipb.P2PSrTePolicy](
			resourcesToBeAdded["p2pSrTePolicies"], localResources.p2pSrTePolicies, printMode, dryRunMode, func(policy *provapipb.P2PSrTePolicy) error {
				_, err := provisioningClient.UpdateP2PSrTePolicy(ctx, &provapipb.UpdateP2PSrTePolicyRequest{
					Policy:       policy,
					AllowMissing: true,
				})
				return err
			})...)
		if err != nil {
			return err
		}

		// Add candidate paths after all policies are created, so parent policies are guaranteed to exist.
		return errors.Join(createRemoteResources[*provapipb.P2PSrTePolicyCandidatePath](
			resourcesToBeAdded["p2pSrTePolicyCandidatePaths"], localResources.p2pSrTePolicyCandidatePaths, printMode, dryRunMode, func(path *provapipb.P2PSrTePolicyCandidatePath) error {
				_, err := provisioningClient.UpdateP2PSrTePolicyCandidatePath(ctx, &provapipb.UpdateP2PSrTePolicyCandidatePathRequest{
					Path:         path,
					AllowMissing: true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.Downtime](
			resourcesToBeAdded["downtimes"], localResources.downtimes, printMode, dryRunMode, func(downtime *provapipb.Downtime) error {
				_, err := provisioningClient.UpdateDowntime(ctx, &provapipb.UpdateDowntimeRequest{
					Downtime:     downtime,
					AllowMissing: true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.ProtectionAssociationGroup](
			resourcesToBeAdded["protectionAssociationGroups"], localResources.protectionAssociationGroups, printMode, dryRunMode, func(protectionAssociationGroup *provapipb.ProtectionAssociationGroup) error {
				_, err := provisioningClient.UpdateProtectionAssociationGroup(ctx, &provapipb.UpdateProtectionAssociationGroupRequest{
					ProtectionAssociationGroup: protectionAssociationGroup,
					AllowMissing:               true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.DisjointAssociationGroup](
			resourcesToBeAdded["disjointAssociationGroups"], localResources.disjointAssociationGroups, printMode, dryRunMode, func(disjointAssociationGroup *provapipb.DisjointAssociationGroup) error {
				_, err := provisioningClient.UpdateDisjointAssociationGroup(ctx, &provapipb.UpdateDisjointAssociationGroupRequest{
					DisjointAssociationGroup: disjointAssociationGroup,
					AllowMissing:             true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.Link](
			resourcesToBeAdded["links"], localResources.links, printMode, dryRunMode, func(link *provapipb.Link) error {
				_, err := provisioningClient.UpdateLink(ctx, &provapipb.UpdateLinkRequest{
					Link:         link,
					AllowMissing: true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.GeographicRegion](
			resourcesToBeAdded["geographicRegions"], localResources.geographicRegions, printMode, dryRunMode, func(geographicRegion *provapipb.GeographicRegion) error {
				_, err := provisioningClient.UpdateGeographicRegion(ctx, &provapipb.UpdateGeographicRegionRequest{
					GeographicRegion: geographicRegion,
					AllowMissing:     true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.EmissionsLimit](
			resourcesToBeAdded["emissionsLimits"], localResources.emissionsLimits, printMode, dryRunMode, func(emissionsLimit *provapipb.EmissionsLimit) error {
				_, err := provisioningClient.UpdateEmissionsLimit(ctx, &provapipb.UpdateEmissionsLimitRequest{
					EmissionsLimit: emissionsLimit,
					AllowMissing:   true,
				})
				return err
			})...)
	})
	p.Go(func() error {
		return errors.Join(createRemoteResources[*provapipb.EmissionsTarget](
			resourcesToBeAdded["emissionsTargets"], localResources.emissionsTargets, printMode, dryRunMode, func(emissionsTarget *provapipb.EmissionsTarget) error {
				_, err := provisioningClient.UpdateEmissionsTarget(ctx, &provapipb.UpdateEmissionsTargetRequest{
					EmissionsTarget: emissionsTarget,
					AllowMissing:    true,
				})
				return err
			})...)
	})

	errs = append(errs, p.Wait())
	return errors.Join(errs...)
}

func ProvisioningList(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, provisioningAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()

	provisioningClient := provapipb.NewProvisioningClient(conn)
	remoteResources, err := ProvisioningResourcesFromRemote(appCtx.Context, provisioningClient)
	if err != nil {
		return err
	}

	fmt.Printf("\nfound remote resources:\n%s\n", remoteResources.MarshalledString(marshaller))

	return nil
}

type deleteProvisioningParams struct {
	p2pSrTePolicies             []string
	p2pSrTePolicyCandidatePaths []string
	downtimes                   []string
	protectionAssociationGroups []string
	disjointAssociationGroups   []string
	links                       []string
	geographicRegions           []string
	emissionsLimits             []string
	emissionsTargets            []string

	printMode  bool
	dryRunMode bool
	client     provapipb.ProvisioningClient
}

func deleteProvisioning(ctx context.Context, params deleteProvisioningParams) error {
	client := params.client
	printMode := params.printMode
	dryRunMode := params.dryRunMode

	// Delete candidate paths before the policies, in case they need to be removed separately.
	p := pool.New().WithErrors()
	if len(params.p2pSrTePolicyCandidatePaths) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.p2pSrTePolicyCandidatePaths, printMode, dryRunMode, func(path string) error {
					_, err := client.DeleteP2PSrTePolicyCandidatePath(ctx, &provapipb.DeleteP2PSrTePolicyCandidatePathRequest{
						Name: path,
					})
					return err
				})...)
		})
	}
	err := p.Wait()
	if err != nil {
		return err
	}

	p = pool.New().WithErrors()
	if len(params.p2pSrTePolicies) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.p2pSrTePolicies, printMode, dryRunMode, func(policy string) error {
					_, err := client.DeleteP2PSrTePolicy(ctx, &provapipb.DeleteP2PSrTePolicyRequest{
						Name: policy,
					})
					return err
				})...)
		})
	}
	if len(params.downtimes) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.downtimes, printMode, dryRunMode, func(downtime string) error {
					_, err := client.DeleteDowntime(ctx, &provapipb.DeleteDowntimeRequest{
						Name: downtime,
					})
					return err
				})...)
		})
	}
	if len(params.protectionAssociationGroups) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.protectionAssociationGroups, printMode, dryRunMode, func(protectionAssociationGroup string) error {
					_, err := client.DeleteProtectionAssociationGroup(ctx, &provapipb.DeleteProtectionAssociationGroupRequest{
						Name: protectionAssociationGroup,
					})
					return err
				})...)
		})
	}
	if len(params.disjointAssociationGroups) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.disjointAssociationGroups, printMode, dryRunMode, func(disjointAssociationGroup string) error {
					_, err := client.DeleteDisjointAssociationGroup(ctx, &provapipb.DeleteDisjointAssociationGroupRequest{
						Name: disjointAssociationGroup,
					})
					return err
				})...)
		})
	}
	if len(params.links) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.links, printMode, dryRunMode, func(link string) error {
					_, err := client.DeleteLink(ctx, &provapipb.DeleteLinkRequest{
						Name: link,
					})
					return err
				})...)
		})
	}
	if len(params.geographicRegions) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.geographicRegions, printMode, dryRunMode, func(geographicRegion string) error {
					_, err := client.DeleteGeographicRegion(ctx, &provapipb.DeleteGeographicRegionRequest{
						Name: geographicRegion,
					})
					return err
				})...)
		})
	}
	if len(params.emissionsLimits) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.emissionsLimits, printMode, dryRunMode, func(emissionsLimit string) error {
					_, err := client.DeleteEmissionsLimit(ctx, &provapipb.DeleteEmissionsLimitRequest{
						Name: emissionsLimit,
					})
					return err
				})...)
		})
	}
	if len(params.emissionsTargets) > 0 {
		p.Go(func() error {
			return errors.Join(deleteRemoteResources(
				params.emissionsTargets, printMode, dryRunMode, func(emissionsTarget string) error {
					_, err := client.DeleteEmissionsTarget(ctx, &provapipb.DeleteEmissionsTargetRequest{
						Name: emissionsTarget,
					})
					return err
				})...)
		})
	}

	return p.Wait()
}

func ProvisioningDeleteAll(appCtx *cli.Context) error {
	ctx := appCtx.Context
	dryRunMode := !appCtx.Bool("execute")
	verboseMode := appCtx.Bool("verbose")
	printMode := dryRunMode || verboseMode

	conn, err := openAPIConnection(appCtx, provisioningAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := provapipb.NewProvisioningClient(conn)
	remoteResources, err := ProvisioningResourcesFromRemote(ctx, client)
	if err != nil {
		return err
	}

	params := deleteProvisioningParams{
		printMode:  printMode,
		dryRunMode: dryRunMode,
		client:     client,

		p2pSrTePolicies:             lo.Keys(remoteResources.p2pSrTePolicies),
		p2pSrTePolicyCandidatePaths: lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths),
		downtimes:                   lo.Keys(remoteResources.downtimes),
		protectionAssociationGroups: lo.Keys(remoteResources.protectionAssociationGroups),
		disjointAssociationGroups:   lo.Keys(remoteResources.disjointAssociationGroups),
		links:                       lo.Keys(remoteResources.links),
		geographicRegions:           lo.Keys(remoteResources.geographicRegions),
		emissionsLimits:             lo.Keys(remoteResources.emissionsLimits),
		emissionsTargets:            lo.Keys(remoteResources.emissionsTargets),
	}

	return deleteProvisioning(appCtx.Context, params)
}

func ProvisioningDelete(appCtx *cli.Context) error {
	ctx := appCtx.Context
	dryRunMode := appCtx.Bool("dry-run")
	verboseMode := appCtx.Bool("verbose")
	resourceNames := appCtx.Args().Slice()
	printMode := dryRunMode || verboseMode

	if len(resourceNames) == 0 {
		return errors.New("no resource names specified")
	}

	conn, err := openAPIConnection(appCtx, provisioningAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := provapipb.NewProvisioningClient(conn)
	remoteResources, err := ProvisioningResourcesFromRemote(ctx, client)
	if err != nil {
		return err
	}

	params := deleteProvisioningParams{
		printMode:  printMode,
		dryRunMode: dryRunMode,
		client:     client,
	}
	params.p2pSrTePolicies = lo.Intersect(resourceNames, lo.Keys(remoteResources.p2pSrTePolicies))
	params.p2pSrTePolicyCandidatePaths = lo.Intersect(resourceNames, lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths))
	params.downtimes = lo.Intersect(resourceNames, lo.Keys(remoteResources.downtimes))
	params.protectionAssociationGroups = lo.Intersect(resourceNames, lo.Keys(remoteResources.protectionAssociationGroups))
	params.disjointAssociationGroups = lo.Intersect(resourceNames, lo.Keys(remoteResources.disjointAssociationGroups))
	params.links = lo.Intersect(resourceNames, lo.Keys(remoteResources.links))
	params.geographicRegions = lo.Intersect(resourceNames, lo.Keys(remoteResources.geographicRegions))
	params.emissionsLimits = lo.Intersect(resourceNames, lo.Keys(remoteResources.emissionsLimits))
	params.emissionsTargets = lo.Intersect(resourceNames, lo.Keys(remoteResources.emissionsTargets))

	deleteResourceNameSet := slices.Concat(
		params.p2pSrTePolicies,
		params.p2pSrTePolicyCandidatePaths,
		params.downtimes,
		params.protectionAssociationGroups,
		params.disjointAssociationGroups,
		params.links,
		params.geographicRegions,
		params.emissionsLimits,
		params.emissionsTargets,
	)

	notFoundNames := lo.Without(resourceNames, deleteResourceNameSet...)

	fmt.Printf("- %d resources not found\n", len(notFoundNames))
	if verboseMode {
		for _, name := range notFoundNames {
			fmt.Println("  -", name)
		}
	}
	fmt.Printf("- %d resources to be deleted\n", len(deleteResourceNameSet))

	if len(deleteResourceNameSet) == 0 {
		return nil
	}

	return deleteProvisioning(appCtx.Context, params)
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
	updateFn func(T) error,
) []error {
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
			fmt.Printf("delete resource: %s\n", key)
		}
		if !dryRunMode {
			if err := deleteFn(key); err != nil && !isNotFoundError(err) {
				errs = append(errs, err)
			}
		}
	}

	return errs
}
