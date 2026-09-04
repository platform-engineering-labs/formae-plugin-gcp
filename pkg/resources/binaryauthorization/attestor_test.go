// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: FSL-1.1-ALv2

//go:build unit

package binaryauthorization

import (
	"reflect"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-gcp/pkg/resources/base"
)

// An attestor read carries three fields no forma ever wrote: etag and
// updateTime at the top level, and delegationServiceAccountEmail nested inside
// userOwnedGrafeasNote. The nested one is the reason this is a transformer -
// hasProviderDefault reaches only top-level fields, and Verify compares a
// nested object as a whole.
func TestAttestorResponseTransformer(t *testing.T) {
	in := map[string]interface{}{
		"name":        "projects/p/attestors/att",
		"description": "probe",
		"etag":        `"TrBzUJAbC6O6"`,
		"updateTime":  "2026-09-03T15:06:33.112856Z",
		"userOwnedGrafeasNote": map[string]interface{}{
			"noteReference":                 "projects/p/notes/n",
			"delegationServiceAccountEmail": "service-1@gcp-sa-binaryauthorization.iam.gserviceaccount.com",
			"publicKeys": []interface{}{
				map[string]interface{}{
					"id": "k1",
					"pkixPublicKey": map[string]interface{}{
						"publicKeyPem":       "-----BEGIN PUBLIC KEY-----\nAAA\n-----END PUBLIC KEY-----",
						"signatureAlgorithm": "ECDSA_P256_SHA256",
					},
				},
			},
		},
	}
	want := map[string]interface{}{
		"name":        "att",
		"description": "probe",
		"userOwnedGrafeasNote": map[string]interface{}{
			"noteReference": "projects/p/notes/n",
			"publicKeys": []interface{}{
				map[string]interface{}{
					"id": "k1",
					"pkixPublicKey": map[string]interface{}{
						"publicKeyPem":       "-----BEGIN PUBLIC KEY-----\nAAA\n-----END PUBLIC KEY-----",
						"signatureAlgorithm": "ECDSA_P256_SHA256",
					},
				},
			},
		},
	}
	got := attestorResponseTransformer(in, base.TransformContext{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// attestors.create omits etag while attestors.get returns it. The transformer
// has to leave both reads looking the same, or a resource verified straight
// after create and the same resource verified on the next sync disagree.
func TestAttestorResponseTransformerCreateAndGetAgree(t *testing.T) {
	create := map[string]interface{}{
		"name":                 "projects/p/attestors/att",
		"updateTime":           "2026-09-03T15:06:33.112856Z",
		"userOwnedGrafeasNote": map[string]interface{}{"noteReference": "projects/p/notes/n"},
	}
	get := map[string]interface{}{
		"name":                 "projects/p/attestors/att",
		"updateTime":           "2026-09-03T15:07:00.000000Z",
		"etag":                 `"sAArrytQi6qw"`,
		"userOwnedGrafeasNote": map[string]interface{}{"noteReference": "projects/p/notes/n"},
	}
	a := attestorResponseTransformer(create, base.TransformContext{})
	b := attestorResponseTransformer(get, base.TransformContext{})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("create %#v\nget    %#v", a, b)
	}
}

// A platform policy's nested structure is echoed back exactly as sent, down to
// the PEM inside a Sigstore authority. Only the name and the two server-owned
// fields are touched: everything else must survive byte for byte, or an
// immutable nested field would disagree with the declaration forever.
func TestPlatformPolicyResponseTransformerLeavesChecksAlone(t *testing.T) {
	gke := map[string]interface{}{
		"imageAllowlist": map[string]interface{}{
			"allowPattern": []interface{}{"gcr.io/formae-test/**"},
		},
		"checkSets": []interface{}{
			map[string]interface{}{
				"displayName": "scoped",
				"scope":       map[string]interface{}{"kubernetesNamespace": "ns"},
				"checks": []interface{}{
					map[string]interface{}{
						"displayName":           "trusted dir",
						"trustedDirectoryCheck": map[string]interface{}{"trustedDirPatterns": []interface{}{"gcr.io/formae-test"}},
					},
					map[string]interface{}{
						"displayName": "vuln",
						"vulnerabilityCheck": map[string]interface{}{
							"maximumFixableSeverity":   "HIGH",
							"maximumUnfixableSeverity": "CRITICAL",
						},
					},
					map[string]interface{}{
						"displayName": "sigstore",
						"sigstoreSignatureCheck": map[string]interface{}{
							"sigstoreAuthorities": []interface{}{
								map[string]interface{}{
									"displayName": "auth1",
									"publicKeySet": map[string]interface{}{
										"publicKeys": []interface{}{
											map[string]interface{}{"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nAAA\n-----END PUBLIC KEY-----"},
										},
									},
								},
							},
						},
					},
				},
			},
			map[string]interface{}{
				"displayName": "default",
				"checks": []interface{}{
					map[string]interface{}{"alwaysDeny": true},
				},
			},
		},
	}
	in := map[string]interface{}{
		"name":        "projects/p/platforms/gke/policies/pol",
		"description": "probe",
		"etag":        `"Rl1g4AxjoGX+"`,
		"updateTime":  "2026-09-03T15:07:50.378106Z",
		"gkePolicy":   gke,
	}
	want := map[string]interface{}{
		"name":        "pol",
		"description": "probe",
		"gkePolicy":   gke,
	}
	got := platformPolicyResponseTransformer(in, base.TransformContext{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// The two native ID shapes have to survive build -> parse -> build unchanged.
// A platform policy's id carries a "platforms/gke" segment the generic
// key/value path parser would walk straight past, and an id that does not
// round-trip addresses the wrong collection on the next read.
func TestNativeIDRoundTripIsIdentity(t *testing.T) {
	for _, nativeID := range []string{
		"projects/development-477117/attestors/formae-test-ba-att-abcd1234",
		"projects/development-477117/platforms/gke/policies/formae-test-ba-pp-abcd1234",
	} {
		ctx, err := parseBinaryAuthorizationNativeID(nativeID)
		if err != nil {
			t.Fatalf("%s: %v", nativeID, err)
		}
		got := extractBinaryAuthorizationNativeID(map[string]interface{}{}, ctx)
		if got != nativeID {
			t.Errorf("got %q, want %q", got, nativeID)
		}
		if want := binaryAuthorizationPathBuilder(ctx); want != "/"+nativeID {
			t.Errorf("path builder gave %q, want %q", want, "/"+nativeID)
		}
	}
}

func TestParseBinaryAuthorizationNativeIDRejectsGarbage(t *testing.T) {
	for _, bad := range []string{
		"",
		"attestors/att",
		"projects/p/attestors",
		"projects/p/notes/n",
		"projects/p/platforms/gke/policies/pol/extra",
		"projects/p/platforms/gke/attestors/att",
	} {
		if _, err := parseBinaryAuthorizationNativeID(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// Nothing in this API returns an Operation, so the extractor must never claim
// one - a false positive would send Create down the async path, where it
// reports in-progress with an empty request id and polls the bare base URL.
func TestExtractOperationNameNeverMatchesAResource(t *testing.T) {
	if got := extractOperationName(map[string]interface{}{"name": "projects/p/attestors/att"}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if got := extractOperationName(map[string]interface{}{}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
