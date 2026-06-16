#!/usr/bin/env sh
set -eu

WEBUI_URL="${WEBUI_URL:-http://127.0.0.1:5050}"
WEBUI_USER="${WEBUI_USER:-admin}"
WEBUI_PASSWORD="${WEBUI_PASSWORD:-free5gc}"

UE_ID="${UE_ID:-imsi-999020000000001}"
PLMN_ID="${PLMN_ID:-99902}"
GPSI="${GPSI:-msisdn-0900000000}"
PERMANENT_KEY="${PERMANENT_KEY:-465B5CE8B199B49FAA5F0A2EE238A6BC}"
OPC="${OPC:-E8ED289DEBA952E4283B54E88E6183CA}"
SEQUENCE_NUMBER="${SEQUENCE_NUMBER:-16f3b3f70fc2}"
SST="${SST:-1}"
SD="${SD:-000001}"
DNN="${DNN:-internet}"

SNSSAI_KEY="$(printf "%02d%s" "$SST" "$SD")"
payload_file="$(mktemp "${TMPDIR:-/tmp}/packetrusher-subscriber.XXXXXX.json")"
trap 'rm -f "$payload_file"' EXIT

token="$(
	curl -sS -X POST "$WEBUI_URL/api/login" \
		-H 'Content-Type: application/json' \
		-d "{\"username\":\"$WEBUI_USER\",\"password\":\"$WEBUI_PASSWORD\"}" |
		sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p'
)"

if [ -z "$token" ]; then
	echo "failed to login to free5GC WebUI at $WEBUI_URL" >&2
	exit 1
fi

cat >"$payload_file" <<JSON
{
  "plmnID": "$PLMN_ID",
  "ueId": "$UE_ID",
  "AuthenticationSubscription": {
    "authenticationMethod": "5G_AKA",
    "permanentKey": {
      "permanentKeyValue": "$PERMANENT_KEY",
      "encryptionKey": 0,
      "encryptionAlgorithm": 0
    },
    "sequenceNumber": "$SEQUENCE_NUMBER",
    "milenage": {
      "op": {
        "opValue": "",
        "encryptionKey": 0,
        "encryptionAlgorithm": 0
      }
    },
    "opc": {
      "opcValue": "$OPC",
      "encryptionKey": 0,
      "encryptionAlgorithm": 0
    },
    "authenticationManagementField": "8000"
  },
  "AccessAndMobilitySubscriptionData": {
    "gpsis": ["$GPSI"],
    "subscribedUeAmbr": {
      "uplink": "1000 Kbps",
      "downlink": "1000 Kbps"
    },
    "nssai": {
      "defaultSingleNssais": [{"sst": $SST, "sd": "$SD"}],
      "singleNssais": [{"sst": $SST, "sd": "$SD"}]
    }
  },
  "SessionManagementSubscriptionData": [
    {
      "singleNssai": {"sst": $SST, "sd": "$SD"},
      "dnnConfigurations": {
        "$DNN": {
          "pduSessionTypes": {
            "defaultSessionType": "IPV4",
            "allowedSessionTypes": ["IPV4"]
          },
          "sscModes": {
            "defaultSscMode": "SSC_MODE_1",
            "allowedSscModes": ["SSC_MODE_1"]
          },
          "5gQosProfile": {
            "5qi": 9,
            "arp": {
              "priorityLevel": 8,
              "preemptCap": "",
              "preemptVuln": ""
            },
            "priorityLevel": 8
          },
          "sessionAmbr": {
            "uplink": "1000 Kbps",
            "downlink": "1000 Kbps"
          }
        }
      }
    }
  ],
  "SmfSelectionSubscriptionData": {
    "subscribedSnssaiInfos": {
      "$SNSSAI_KEY": {
        "dnnInfos": [{"dnn": "$DNN"}]
      }
    }
  },
  "AmPolicyData": {
    "subscCats": ["free5gc"]
  },
  "SmPolicyData": {
    "smPolicySnssaiData": {
      "$SNSSAI_KEY": {
        "snssai": {"sst": $SST, "sd": "$SD"},
        "smPolicyDnnData": {
          "$DNN": {"dnn": "$DNN"}
        }
      }
    }
  },
  "FlowRules": null,
  "QosFlows": null,
  "ChargingDatas": null
}
JSON

curl -sS -X POST "$WEBUI_URL/api/subscriber/$UE_ID/$PLMN_ID" \
	-H "Token: $token" \
	-H 'Content-Type: application/json' \
	--data-binary "@$payload_file"

echo
echo "Seeded PacketRusher subscriber: $UE_ID / PLMN $PLMN_ID / S-NSSAI ${SST}-${SD} / DNN $DNN"
