#!/usr/bin/env bash

TMP_SUBDOMAIN="tmp-`date +%Y-%m-%d--%H-%M-%S--%N--$RANDOM$RANDOM`.ya"
TMP_DOMAIN="$TMP_SUBDOMAIN.$DOMAIN"

go env
go build -v
echo "Tmp domain: $TMP_DOMAIN"

#########
### A ###
#########

echo "Add record"
./ypdd --sync --ttl 914 $DOMAIN add $TMP_SUBDOMAIN A 127.1.2.3 || exit 1

echo
echo "Get record list"
LINE=`./ypdd $DOMAIN list | grep $TMP_SUBDOMAIN`
echo "$LINE"
[ -n "$LINE" ] || exit 1
echo "$LINE" | grep -q 127.1.2.3 || exit 1 # Check content
echo "$LINE" | grep -q 914 || exit 1 # Check TTL

echo
echo "nslookup"
LOOKUP=`nslookup $TMP_DOMAIN dns1.yandex.net`
echo "$LOOKUP"
echo "$LOOKUP" | grep -q 127.1.2.3 || exit 1

echo "Del record"
ID=`echo "$LINE" | cut -d ' ' -f 1`
./ypdd $DOMAIN del $ID || exit 1

##########
### MX ###
##########

echo
echo "Add MX"
./ypdd --sync --ttl 914 $DOMAIN add $TMP_SUBDOMAIN MX 112 test.mx.record. || exit 1
echo
echo "Get record list"
LINE=`./ypdd $DOMAIN list | grep $TMP_SUBDOMAIN`
echo "$LINE"
[ -n "$LINE" ] || exit 1
echo "$LINE" | grep -q test.mx.record. || exiq 1 # Check content
echo "$LINE" | grep -q 914 || exit 1 # Check TTL
echo "$LINE" | grep -q 112 || exit 1 # Check PRIORITY

echo
echo "nslookup"
LOOKUP=`nslookup -type=mx $TMP_DOMAIN dns1.yandex.net`
echo "$LOOKUP"
echo "$LOOKUP" | grep -q test.mx.record. || exit 1
echo "$LOOKUP" | grep -q 112 || exit 1 # Check priority

echo "Del record"
ID=`echo "$LINE" | cut -d ' ' -f 1`
./ypdd $DOMAIN del $ID || exit 1

###########
### SRV ###
###########
echo
echo "Add SRV"
./ypdd --sync --ttl 914 $DOMAIN add $TMP_SUBDOMAIN SRV 112 312 1561 test.srv.record. || exit 1
echo
echo "Get record list"
LINE=`./ypdd $DOMAIN list | grep $TMP_SUBDOMAIN`
echo "$LINE"
[ -n "$LINE" ] || exit 1
echo "$LINE" | grep -q test.srv.record. || exit 1 # Check content
echo "$LINE" | grep -q 914 || exit 1 # Check TTL
echo "$LINE" | grep -q 112 || exit 1 # Check PRIORITY

echo
echo "nslookup"
LOOKUP=`nslookup -type=srv $TMP_DOMAIN dns1.yandex.net`
echo "$LOOKUP"
echo "$LOOKUP" | grep -q test.srv.record. || exit 1 # Check content
echo "$LOOKUP" | grep -q 112 || exit 1 # Check priority
echo "$LOOKUP" | grep -q 312 || exit 1 # Check weight
echo "$LOOKUP" | grep -q 1561 || exit 1 # Check port

echo "Del record"
ID=`echo "$LINE" | cut -d ' ' -f 1`
./ypdd $DOMAIN del $ID || exit 1
