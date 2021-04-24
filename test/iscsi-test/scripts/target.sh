#!/bin/bash

tgname="iqn.com.example.test"

mkdir -p /tmp/tgtimgs
img=$(mktemp -p /tmp/tgtimgs)
tid=$(cat /dev/urandom | tr -cd '0-9' | head -c 4)
tsub=$(cat /dev/urandom | tr -cd 'a-f0-9' | head -c 4)

echo "IMG=$img"
dd if=/dev/zero of=$img bs=1M count=4
ldev=$(losetup --find --show $img)
echo "DEV=$ldev"
echo "TID=$tid"
echo "TGT=$tgname:$tsub"
hostname -i

#/etc/init.d/tgt start
tgtadm --lld iscsi --op new  --mode target      --tid $tid --targetname $tgname:$tsub
tgtadm --lld iscsi --op new  --mode logicalunit --tid $tid --lun 1 --backing-store $ldev
tgtadm --lld iscsi --op bind --mode target      --tid $tid --initiator-address ALL