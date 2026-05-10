# grpcurl RunChecks JSON as main input; --slurpfile META runchecks-live-meta.json

def dict_reason_ru:
  {
    "REASON_CODE_UNSPECIFIED": "код не указан",
    "VALID_STORAGE_PROOF": "валидное хранилище (proof)",
    "IP_NOT_FOUND": "IP не найден",
    "NOT_FOUND": "не найдено",
    "UNAVAILABLE_PROVIDER": "провайдер недоступен",
    "CANT_CREATE_PEER": "не удалось создать peer",
    "UNKNOWN_PEER": "неизвестный peer",
    "PING_FAILED": "пинг не прошёл",
    "INVALID_BAG_ID": "неверный bag_id",
    "FAILED_INITIAL_PING": "начальный пинг не прошёл",
    "GET_INFO_FAILED": "get_info не удался",
    "INVALID_HEADER": "неверный заголовок",
    "CANT_GET_PIECE": "не удалось получить фрагмент",
    "CANT_PARSE_BOC": "разбор BOC",
    "PROOF_CHECK_FAILED": "проверка proof не прошла"
  };

def reason_key: if type == "string" then . else tostring end;

def reason_ru:
  reason_key as $k
  | dict_reason_ru[$k]
    // (
        {
          "0": "код не указан",
          "1": "валидное хранилище (proof)",
          "101": "IP не найден",
          "102": "не найдено",
          "103": "провайдер недоступен",
          "104": "не удалось создать peer",
          "105": "неизвестный peer",
          "201": "пинг не прошёл",
          "202": "неверный bag_id",
          "203": "начальный пинг не прошёл",
          "301": "get_info не удался",
          "302": "неверный заголовок",
          "401": "не удалось получить фрагмент",
          "402": "разбор BOC",
          "403": "проверка proof не прошла"
        }[$k] // $k
      );

($META[0] // {providers: []}) as $meta
| ($meta.providers
    | map(select(.providerPubkey != null and .providerPubkey != ""))
    | map({(.providerPubkey): .})
    | add // {}) as $geo
| (.results // []) as $results
| "=== По провайдерам: регион (по IP storage) → итоги по контрактам ===",
  (
    ($results | group_by(.providerPubkey) | sort_by(.[0].providerPubkey))[]
    | (.[0].providerPubkey) as $pk
    | ($geo[$pk] // null) as $g
    | (
        if ($g != null) and (($g | type) == "object") then
          (($g.countryIso // "?") | tostring)
          + " · "
          + (($g.country // "?") | tostring)
          + (
              if (($g.city // "") | tostring | length) > 0
              then " · " + ($g.city | tostring)
              else "" end
            )
          + " · "
          + (($g.storageIp // "?") | tostring)
          + (
              if (($g.geoLookupError // "") | tostring | length) > 0
              then " · ⚠ гео: " + (($g.geoLookupError | tostring | .[0:80]))
              else "" end
            )
        else
          ("регион неизвестен (нет meta или pubkey) · " + ($pk | .[0:24]) + "…")
        end
      ) as $region
    | (
        map(.reasonCode | reason_ru)
        | group_by(.)
        | map(.[0] as $lab | "\($lab) × \(length)")
        | join(", ")
      ) as $reasons
    | "  \($region)\n     \($reasons)"
  ),
  ""
