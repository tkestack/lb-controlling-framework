#!/usr/bin/env bash

mkdir -p output

for d in `go list tkestack.io/lb-controlling-framework/pkg/... | grep -v pkg/apis | grep -v pkg/client-go`
do
   go test -covermode=atomic -coverprofile=cover.out $d
   if [ $? -ne 0 ]
   then
        exit 1
   fi
   if [ -f cover.out ]
   then
      cat cover.out >> output/coverage.txt
      rm cover.out
   fi
done

cat output/coverage.txt
