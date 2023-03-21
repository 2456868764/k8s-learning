#!/bin/sh
/app/httpbin &
envoy -c /etc/service-envoy.yaml --service-cluster service${SERVICE_NAME}