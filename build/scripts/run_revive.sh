#!/usr/bin/env bash

REVIVE_RESULT=output/revive.out

mkdir -p output

revive -config build/linter/revive.toml -exclude pkg/apis/... -exclude pkg/client-go/... pkg/... > $REVIVE_RESULT

invalid=`wc -l < $REVIVE_RESULT`; if [ $invalid -gt 0 ]; then cat $REVIVE_RESULT && exit 1; fi