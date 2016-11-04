#!/usr/bin/env bash

TMP_SUBDOMAIN="tmp-`date +%Y-%m-%d--%H-%M-%S--%N--$RANDOM$RANDOM`"
TMP_DOMAIN="$TMP_SUBDOMAIN.$DOMAIN"

go env
go build -v
echo "Tmp domain: $TMP_DOMAIN"

echo "Add record"
./ypdd --ttl 60 $DOMAIN add $TMP_SUBDOMAIN A 127.1.2.3

echo
echo "Get record list"
LINE=`./ypdd $DOMAIN list | grep $TMP_SUBDOMAIN`
echo "$LINE"

echo
echo "nslookup"
LOOKUP=`nslookup $TMP_DOMAIN`
echo "$LOOKUP"
echo "$LOOKUP" | grep -q 127.1.2.3

echo "Del record"
ID=`echo "$LINE" | cut -d ' ' -f 1`
./ypdd $DOMAIN del $ID
