package dnsprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	awstypes "github.com/aws/aws-sdk-go-v2/service/route53/types"
	alidns "github.com/go-acme/alidns-20150109/v4/client"
	dnspod "github.com/nrdcg/dnspod-go"
)

func newRecordWriter(provider string, values map[string]string) (recordWriter, error) {
	values, err := ProviderCredentials(provider, values)
	if err != nil {
		return nil, err
	}
	switch provider {
	case "alidns":
		client, err := alidns.NewClient(new(openapi.Config).
			SetRegionId("cn-hangzhou").
			SetAccessKeyId(values["ALICLOUD_ACCESS_KEY"]).
			SetAccessKeySecret(values["ALICLOUD_SECRET_KEY"]))
		if err != nil {
			return nil, err
		}
		return &aliRecordWriter{client: client}, nil
	case "dnspod":
		client := dnspod.NewClient(dnspod.CommonParams{LoginToken: values["DNSPOD_API_KEY"], Format: "json"})
		client.HTTPClient = &http.Client{Timeout: 15 * time.Second}
		return &dnspodRecordWriter{client: client}, nil
	case "cloudflare":
		return &cloudflareRecordWriter{token: values["CLOUDFLARE_DNS_API_TOKEN"], client: &http.Client{Timeout: 15 * time.Second}}, nil
	case "route53":
		client := route53.New(route53.Options{
			Region: values["AWS_REGION"],
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
				values["AWS_ACCESS_KEY_ID"], values["AWS_SECRET_ACCESS_KEY"], "")),
		})
		return &route53RecordWriter{client: client}, nil
	default:
		return nil, errors.New("不支持的 DNS 平台")
	}
}

type aliRecordWriter struct{ client *alidns.Client }

func (w *aliRecordWriter) List(ctx context.Context, zone, rr string) ([]dnsRecord, error) {
	var records []dnsRecord
	for page := int64(1); ; page++ {
		response, err := alidns.DescribeDomainRecordsWithContext(ctx, w.client,
			new(alidns.DescribeDomainRecordsRequest).SetDomainName(zone).SetRRKeyWord(rr).SetPageNumber(page).SetPageSize(500),
			new(dara.RuntimeOptions))
		if err != nil {
			return nil, err
		}
		if response.Body == nil {
			return nil, errors.New("阿里云 DNS 返回空结果")
		}
		if response.Body.DomainRecords != nil {
			for _, record := range response.Body.DomainRecords.Record {
				if dara.StringValue(record.RR) == rr {
					records = append(records, dnsRecord{ID: dara.StringValue(record.RecordId), Type: dara.StringValue(record.Type), Value: dara.StringValue(record.Value)})
				}
			}
		}
		if int64(len(records)) >= dara.Int64Value(response.Body.TotalCount) || page*500 >= dara.Int64Value(response.Body.TotalCount) {
			return records, nil
		}
	}
}

func (w *aliRecordWriter) Add(ctx context.Context, zone, rr, recordType, value string) (string, error) {
	response, err := alidns.AddDomainRecordWithContext(ctx, w.client,
		new(alidns.AddDomainRecordRequest).SetDomainName(zone).SetRR(rr).SetType(recordType).SetValue(value),
		new(dara.RuntimeOptions))
	if err != nil {
		return "", err
	}
	if response.Body == nil || dara.StringValue(response.Body.RecordId) == "" {
		return "", errors.New("阿里云 DNS 未返回记录 ID")
	}
	return dara.StringValue(response.Body.RecordId), nil
}

func (w *aliRecordWriter) Delete(ctx context.Context, id string) error {
	_, err := alidns.DeleteDomainRecordWithContext(ctx, w.client,
		new(alidns.DeleteDomainRecordRequest).SetRecordId(id), new(dara.RuntimeOptions))
	return err
}

type dnspodRecordWriter struct {
	client   *dnspod.Client
	domainID string
}

func (w *dnspodRecordWriter) List(ctx context.Context, zone, rr string) ([]dnsRecord, error) {
	domains, _, err := w.client.Domains.List()
	if err != nil {
		return nil, err
	}
	for _, domain := range domains {
		if strings.EqualFold(domain.Name, zone) {
			w.domainID = domain.ID.String()
			break
		}
	}
	if w.domainID == "" {
		return nil, errors.New("DNSPod 中未找到托管区域")
	}
	items, _, err := w.client.Records.List(w.domainID, rr)
	if err != nil {
		return nil, err
	}
	records := make([]dnsRecord, 0, len(items))
	for _, item := range items {
		if item.Name == rr {
			records = append(records, dnsRecord{ID: item.ID, Type: item.Type, Value: item.Value})
		}
	}
	return records, nil
}

func (w *dnspodRecordWriter) Add(_ context.Context, _, rr, recordType, value string) (string, error) {
	record, _, err := w.client.Records.Create(w.domainID, dnspod.Record{
		Name: rr, Type: recordType, Value: value, Line: "默认",
	})
	return record.ID, err
}

func (w *dnspodRecordWriter) Delete(_ context.Context, id string) error {
	_, err := w.client.Records.Delete(w.domainID, id)
	return err
}

type cloudflareRecordWriter struct {
	token  string
	client *http.Client
	zoneID string
}

func (w *cloudflareRecordWriter) request(ctx context.Context, method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, "https://api.cloudflare.com/client/v4/"+path, payload)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+w.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := w.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var result struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return err
	}
	if response.StatusCode >= 400 || !result.Success {
		if len(result.Errors) > 0 {
			return fmt.Errorf("Cloudflare DNS: %s", result.Errors[0].Message)
		}
		return fmt.Errorf("Cloudflare DNS HTTP %d", response.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(result.Result, out)
	}
	return nil
}

func (w *cloudflareRecordWriter) List(ctx context.Context, zone, rr string) ([]dnsRecord, error) {
	var zones []struct {
		ID string `json:"id"`
	}
	if err := w.request(ctx, http.MethodGet, "zones?name="+url.QueryEscape(zone), nil, &zones); err != nil {
		return nil, err
	}
	if len(zones) != 1 {
		return nil, errors.New("Cloudflare 中未找到唯一托管区域")
	}
	w.zoneID = zones[0].ID
	name := rr + "." + zone
	var records []dnsRecord
	for page := 1; ; page++ {
		var items []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		path := fmt.Sprintf("zones/%s/dns_records?name=%s&page=%d&per_page=100", url.PathEscape(w.zoneID), url.QueryEscape(name), page)
		if err := w.request(ctx, http.MethodGet, path, nil, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			if strings.EqualFold(item.Name, name) {
				records = append(records, dnsRecord{ID: item.ID, Type: item.Type, Value: item.Content})
			}
		}
		if len(items) < 100 {
			return records, nil
		}
	}
}

func (w *cloudflareRecordWriter) Add(ctx context.Context, zone, rr, recordType, value string) (string, error) {
	var result struct {
		ID string `json:"id"`
	}
	err := w.request(ctx, http.MethodPost, "zones/"+url.PathEscape(w.zoneID)+"/dns_records",
		map[string]any{"type": recordType, "name": rr + "." + zone, "content": value, "ttl": 600, "proxied": false}, &result)
	return result.ID, err
}

func (w *cloudflareRecordWriter) Delete(ctx context.Context, id string) error {
	return w.request(ctx, http.MethodDelete, "zones/"+url.PathEscape(w.zoneID)+"/dns_records/"+url.PathEscape(id), nil, nil)
}

type route53RecordWriter struct {
	client *route53.Client
	zoneID string
	name   string
	type_  awstypes.RRType
	values []string
	ttl    int64
}

func (w *route53RecordWriter) List(ctx context.Context, zone, rr string) ([]dnsRecord, error) {
	response, err := w.client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{DNSName: aws.String(zone + ".")})
	if err != nil {
		return nil, err
	}
	for _, item := range response.HostedZones {
		if strings.EqualFold(aws.ToString(item.Name), zone+".") && (item.Config == nil || !item.Config.PrivateZone) {
			w.zoneID = aws.ToString(item.Id)
			break
		}
	}
	if w.zoneID == "" {
		return nil, errors.New("Route53 中未找到公网托管区域")
	}
	w.name = rr + "." + zone + "."
	sets, err := w.client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(w.zoneID), StartRecordName: aws.String(w.name), MaxItems: aws.Int32(300),
	})
	if err != nil {
		return nil, err
	}
	var records []dnsRecord
	for _, set := range sets.ResourceRecordSets {
		if !strings.EqualFold(aws.ToString(set.Name), w.name) {
			break
		}
		if set.Type == awstypes.RRTypeA || set.Type == awstypes.RRTypeCname {
			if set.AliasTarget != nil || set.SetIdentifier != nil {
				return nil, errors.New("Route53 已有复杂路由记录，请手动处理")
			}
			w.type_ = set.Type
			w.ttl = aws.ToInt64(set.TTL)
			for _, item := range set.ResourceRecords {
				value := aws.ToString(item.Value)
				records = append(records, dnsRecord{ID: value, Type: string(set.Type), Value: value})
				w.values = append(w.values, value)
			}
		}
	}
	if sets.IsTruncated && strings.EqualFold(aws.ToString(sets.NextRecordName), w.name) {
		return nil, errors.New("Route53 同名记录过多，请手动处理")
	}
	return records, nil
}

func (w *route53RecordWriter) change(ctx context.Context, action awstypes.ChangeAction) error {
	values := make([]awstypes.ResourceRecord, len(w.values))
	for i, value := range w.values {
		values[i] = awstypes.ResourceRecord{Value: aws.String(value)}
	}
	if w.ttl == 0 {
		w.ttl = 600
	}
	_, err := w.client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(w.zoneID),
		ChangeBatch: &awstypes.ChangeBatch{Changes: []awstypes.Change{{
			Action:            action,
			ResourceRecordSet: &awstypes.ResourceRecordSet{Name: aws.String(w.name), Type: w.type_, TTL: aws.Int64(w.ttl), ResourceRecords: values},
		}}},
	})
	return err
}

func (w *route53RecordWriter) Add(ctx context.Context, _, _, recordType, value string) (string, error) {
	if w.type_ == "" {
		w.type_ = awstypes.RRType(recordType)
	}
	w.values = append(w.values, value)
	if err := w.change(ctx, awstypes.ChangeActionUpsert); err != nil {
		w.values = w.values[:len(w.values)-1]
		return "", err
	}
	return value, nil
}

func (w *route53RecordWriter) Delete(ctx context.Context, id string) error {
	index := -1
	for i, value := range w.values {
		if value == id {
			index = i
			break
		}
	}
	if index < 0 {
		return nil
	}
	previous := append([]string(nil), w.values...)
	w.values = append(w.values[:index], w.values[index+1:]...)
	action := awstypes.ChangeActionUpsert
	if len(w.values) == 0 {
		w.values = previous
		action = awstypes.ChangeActionDelete
	}
	if err := w.change(ctx, action); err != nil {
		w.values = previous
		return err
	}
	if action == awstypes.ChangeActionDelete {
		w.values = nil
	}
	return nil
}
