#!/usr/bin/env bash

TMP_SUBDOMAIN="tmp-`date +%Y-%m-%d--%H-%M-%S--%N--$RANDOM$RANDOM`"
TMP_DOMAIN="$TMP_SUBDOMAIN.$DOMAIN"

go env
go build -v
echo "Tmp domain: $TMP_DOMAIN"

./ypdd --ttl 60 $DOMAIN add $TMP_SUBDOMAIN A 127.0.0.1
LINE=`./ypdd $DOMAIN list | grep $TMP_SUBDOMAIN`
echo "$LINE"
ID=`echo "$LINE" | cut -d ' ' -f 1`
./ypdd $DOMAIN del $ID
