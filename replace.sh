#!/bin/bash

git clone https://github.com/morgangallant/mglogs.git /root/mglogs
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