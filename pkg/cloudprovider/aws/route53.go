package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	"github.com/aws/aws-sdk-go/service/route53"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/klog/v2"
)

func route53OwnerValue(resource, ns, name string) string {
	return "\"heritage=aws-global-accelerator-controller," + resource + "/" + ns + "/" + name + "\""
}

func (a *AWS) EnsureRoute53ForService(
	ctx context.Context,
	svc *corev1.Service,
	lbIngress *corev1.LoadBalancerIngress,
	hostnames []string,
) (bool, time.Duration, error) {
	return a.ensureRoute53(ctx, lbIngress, hostnames, "service", svc.Namespace, svc.Name)
}

func (a *AWS) EnsureRoute53ForIngress(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	lbIngress *corev1.LoadBalancerIngress,
	hostnames []string,
) (bool, time.Duration, error) {
	return a.ensureRoute53(ctx, lbIngress, hostnames, "ingress", ingress.Namespace, ingress.Name)
}

func (a *AWS) ensureRoute53(
	ctx context.Context,
	lbIngress *corev1.LoadBalancerIngress,
	hostnames []string,
	resource, ns, name string,
) (bool, time.Duration, error) {
	// Get Global Accelerator
	accelerators, err := a.ListGlobalAcceleratorByHostname(ctx, lbIngress.Hostname, resource, ns, name)
	if err != nil {
		klog.Error(err)
		return false, 0, err
	}
	if len(accelerators) > 1 {
		err := fmt.Errorf("Too many Global Accelerators for %s", lbIngress.Hostname)
		klog.Error(err)
		return false, 1 * time.Minute, nil
	} else if len(accelerators) == 0 {
		err := fmt.Errorf("Could not find Global Accelerato for %s", lbIngress.Hostname)
		klog.Error(err)
		return false, 1 * time.Minute, nil
	}
	accelerator := accelerators[0]

	created := false
HOSTNAMES:
	for _, hostname := range hostnames {
		// Find hosted zone
		hostedZone, err := a.getHostedZone(ctx, hostname)
		if err != nil {
			klog.Error(err)
			return false, 0, err
		}
		klog.Infof("HostedZone is %s", *hostedZone.Id)

		klog.Infof("Finding record sets for HostedZone %s", *hostedZone.Id)
		records, err := a.findOwneredARecordSets(ctx, hostedZone, route53OwnerValue(resource, ns, name))
		if err != nil {
			klog.Error(err)
			return false, 0, err
		}

		if len(records) > 1 {
			err := fmt.Errorf("Too many records for %s", hostname)
			klog.Error(err)
			return false, 0, err
		} else if len(records) == 0 {
			// Create a new record set
			klog.Infof("Creating record for %s with %s", hostname, *accelerator.AcceleratorArn)
			err = a.createMetadataRecordSet(ctx, hostedZone, hostname, resource, ns, name)
			if err != nil {
				klog.Error(err)
				return false, 0, err
			}
			err = a.createRecordSet(ctx, hostedZone, hostname, accelerator)
			if err != nil {
				klog.Error(err)
				return false, 0, err
			}
			created = true
		} else {
			if !needRecordsUpdate(records[0], accelerator) {
				continue HOSTNAMES
			}
			err = a.updateRecordSet(ctx, hostedZone, hostname, accelerator)
			if err != nil {
				klog.Error(err)
				return false, 0, err
			}
			klog.Infof("RecordSet for %s is updated", hostname)
		}
	}

	klog.Infof("All records are synced for %s %s/%s", resource, ns, name)
	return created, 0, nil
}

func (a *AWS) CleanupRecordSet(ctx context.Context, resource, ns, name string) error {
	zones, err := a.listAllHostedZone(ctx)
	if err != nil {
		klog.Error(err)
		return err
	}
	for _, zone := range zones {
		records, err := a.findOwneredARecordSets(ctx, zone, route53OwnerValue(resource, ns, name))
		if err != nil {
			klog.Error(err)
			return err
		}
		for _, record := range records {
			if err := a.deleteRecord(ctx, zone, record); err != nil {
				klog.Error(err)
				return err
			}
			klog.Infof("Record set %s: %s is deleted", *record.Name, *record.Type)
		}
		records, err = a.findOwneredMetadataRecordSets(ctx, zone, route53OwnerValue(resource, ns, name))
		if err != nil {
			klog.Error(err)
			return err
		}
		for _, record := range records {
			if err := a.deleteRecord(ctx, zone, record); err != nil {
				klog.Error(err)
				return err
			}
			klog.Infof("Record set %s: %s is deleted", *record.Name, *record.Type)
		}
	}
	return nil
}

func (a *AWS) findOwneredMetadataRecordSets(ctx context.Context, hostedZone *route53.HostedZone, ownerValue string) ([]*route53.ResourceRecordSet, error) {
	recordSets, err := a.listRecordSets(ctx, hostedZone.Id)
	if err != nil {
		return nil, err
	}
	result := []*route53.ResourceRecordSet{}
	for _, set := range recordSets {
		for _, record := range set.ResourceRecords {
			if *record.Value == ownerValue {
				result = append(result, set)
			}
		}
	}
	return result, nil
}

func (a *AWS) deleteRecord(ctx context.Context, hostedZone *route53.HostedZone, record *route53.ResourceRecordSet) error {
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: hostedZone.Id,
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				&route53.Change{
					Action:            aws.String("DELETE"),
					ResourceRecordSet: record,
				},
			},
		},
	}
	_, err := a.route53.ChangeResourceRecordSetsWithContext(ctx, input)
	return err
}

func (a *AWS) listAllHostedZone(ctx context.Context) ([]*route53.HostedZone, error) {
	input := &route53.ListHostedZonesInput{
		MaxItems: aws.String("100"),
	}
	res, err := a.route53.ListHostedZonesWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.HostedZones, nil
}

func (a *AWS) findOwneredARecordSets(ctx context.Context, hostedZone *route53.HostedZone, ownerValue string) ([]*route53.ResourceRecordSet, error) {
	recordSets, err := a.listRecordSets(ctx, hostedZone.Id)
	if err != nil {
		return nil, err
	}
	hostnames := []string{}
	for _, set := range recordSets {
		for _, record := range set.ResourceRecords {
			if *record.Value == ownerValue {
				hostnames = append(hostnames, *set.Name)
			}
		}
	}
	resultSets := []*route53.ResourceRecordSet{}
	for _, set := range recordSets {
		if hostnameContains(hostnames, *set.Name) && set.AliasTarget != nil {
			resultSets = append(resultSets, set)
		}
	}
	return resultSets, nil
}

func (a *AWS) createRecordSet(ctx context.Context, hostedZone *route53.HostedZone, hostname string, accelerator *globalaccelerator.Accelerator) error {
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: hostedZone.Id,
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				&route53.Change{
					Action: aws.String("CREATE"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(hostname),
						Type: aws.String("A"),
						AliasTarget: &route53.AliasTarget{
							DNSName:              accelerator.DnsName,
							EvaluateTargetHealth: aws.Bool(true),
							// Hosted Zone of Global Accelerator is determined in:
							// https://docs.aws.amazon.com/sdk-for-go/api/service/route53/#AliasTarget
							HostedZoneId: aws.String("Z2BJ6XQ5FK7U4H"),
						},
					},
				},
			},
		},
	}
	_, err := a.route53.ChangeResourceRecordSetsWithContext(ctx, input)
	return err
}

func (a *AWS) createMetadataRecordSet(ctx context.Context, hostedZone *route53.HostedZone, hostname, resource, ns, name string) error {
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: hostedZone.Id,
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				&route53.Change{
					Action: aws.String("CREATE"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(hostname),
						Type: aws.String("TXT"),
						TTL:  aws.Int64(300),
						ResourceRecords: []*route53.ResourceRecord{
							&route53.ResourceRecord{
								Value: aws.String(route53OwnerValue(resource, ns, name)),
							},
						},
					},
				},
			},
		},
	}
	_, err := a.route53.ChangeResourceRecordSetsWithContext(ctx, input)
	return err
}

func (a *AWS) updateRecordSet(ctx context.Context, hostedZone *route53.HostedZone, hostname string, accelerator *globalaccelerator.Accelerator) error {
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: hostedZone.Id,
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				&route53.Change{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(hostname),
						Type: aws.String("A"),
						AliasTarget: &route53.AliasTarget{
							DNSName:              accelerator.DnsName,
							EvaluateTargetHealth: aws.Bool(true),
							// Hosted Zone of Global Accelerator is determined in:
							// https://docs.aws.amazon.com/sdk-for-go/api/service/route53/#AliasTarget
							HostedZoneId: aws.String("Z2BJ6XQ5FK7U4H"),
						},
					},
				},
			},
		},
	}
	_, err := a.route53.ChangeResourceRecordSetsWithContext(ctx, input)
	return err
}

func (a *AWS) listRecordSets(ctx context.Context, hostedZoneID *string) ([]*route53.ResourceRecordSet, error) {
	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId: hostedZoneID,
	}
	res, err := a.route53.ListResourceRecordSetsWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.ResourceRecordSets, nil
}

func (a *AWS) getHostedZone(ctx context.Context, hostname string) (*route53.HostedZone, error) {
	klog.V(4).Infof("Get hosted zone for %s", hostname)
	input := &route53.ListHostedZonesByNameInput{
		DNSName:  aws.String(hostname + "."),
		MaxItems: aws.String("1"),
	}
	res, err := a.route53.ListHostedZonesByNameWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	for _, zone := range res.HostedZones {
		if *zone.Name == hostname+"." {
			return zone, nil
		}
	}
	parent := parentDomain(hostname)
	klog.V(4).Infof("Get hosted zone for %s", parent)
	input = &route53.ListHostedZonesByNameInput{
		DNSName:  aws.String(parent + "."),
		MaxItems: aws.String("1"),
	}
	res, err = a.route53.ListHostedZonesByNameWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	for _, zone := range res.HostedZones {
		if *zone.Name == parent+"." {
			return zone, nil
		}
	}
	return nil, fmt.Errorf("Could not find hosted zone for %s", hostname)
}

func needRecordsUpdate(record *route53.ResourceRecordSet, accelerator *globalaccelerator.Accelerator) bool {
	if record.AliasTarget == nil {
		return true
	}
	if *record.AliasTarget.DNSName != *accelerator.DnsName+"." {
		return true
	}
	return false
}

func parentDomain(hostname string) string {
	slice := strings.Split(hostname, ".")
	return strings.Join(slice[1:], ".")
}

func hostnameContains(hostnames []string, targetName string) bool {
	for _, name := range hostnames {
		if name == targetName {
			return true
		}
	}
	return false
}
