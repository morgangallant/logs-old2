#!/bin/bash

cd /root/mglogs
rm -rf mglogs
go build -o mglogs .
if [ ! -f mglogs ]; then
  echo "compilation failed, terminating"
  exit
fi
echo "deploying"
# systemctl stop mglogs
mv mglogs /root/servers/mglogs
# systemctl start mglogs
echo "done"