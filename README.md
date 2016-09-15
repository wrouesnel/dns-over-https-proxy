# DNS-over-HTTPS proxy

[![Build Status](https://travis-ci.org/wrouesnel/dns-over-https-proxy.svg?branch=master)](https://travis-ci.org/wrouesnel/dns-over-https-proxy) [![Godoc](https://godoc.org/github.com/wrouesnel/dns-over-https-proxy?status.png)](https://godoc.org/github.com/wrouesnel/dns-over-https-proxy)

An implementation of a forwarding DNS proxy for using Google's DNS-over-HTTPS
service with conventional applications.

Currently does no caching or particularly sensible parsing, and supports only
A and AAAA records (as no API to convert them to Go-DNS format is yet written,
and the Google API is still in flux).

## Usage
Just run it!

By default it binds to port 53, so if you have a local resolver it will fail to
start. You can test it by binding to a high port and using dig like so:

```
dns-over-https-proxy -debug=true -address=127.0.0.1:8500 -log.level=debug
```

and then running dig will produce output similar to the below:
```
$ dig -p 8500 @127.0.0.1 google.com
; <<>> DiG 9.9.5-11ubuntu1.3-Ubuntu <<>> -p 8500 @127.0.0.1 google.com
; (1 server found)
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 54973
;; flags: qr rd ra; QUERY: 1, ANSWER: 6, AUTHORITY: 0, ADDITIONAL: 0

;; QUESTION SECTION:
;google.com.			IN	A

;; ANSWER SECTION:
google.com.		299	IN	A	74.125.23.100
google.com.		299	IN	A	74.125.23.113
google.com.		299	IN	A	74.125.23.101
google.com.		299	IN	A	74.125.23.102
google.com.		299	IN	A	74.125.23.138
google.com.		299	IN	A	74.125.23.139

;; Query time: 1302 msec
;; SERVER: 127.0.0.1#8500(127.0.0.1)
;; WHEN: Fri Apr 15 02:26:09 AEST 2016
;; MSG SIZE  rcvd: 184

```

# License #

[Apache License, version 2.0](http://www.apache.org/licenses/LICENSE-2.0).

# Thanks #

- the powerful Go [dns](https://github.com/miekg/dns) library by [Miek Gieben](https://github.com/miekg)
- [dns-reverse-proxy](https://github.com/StalkR/dns-reverse-proxy) on which this code was originally derived.

