#!/bin/bash

# $1: hostname -i
# $2: iqn.xxxx.:$2

tgname="iqn.com.example.test"

#/etc/init.d/iscsid start
iscsiadm -m discovery -t sendtargets -p $1 -l
iscsiadm -m node -T  -p $tgname:$2 $1 --login
iscsiadm -m session -P3