# Telemetry query engine third-party notices

This file records the upstream notices referenced by `deploy/query-parsers.lock.json`. It covers the reused query engines listed below; it is not an inventory of every Argus dependency.

| Module | Version | Upstream revision | License |
| --- | --- | --- | --- |
| `github.com/prometheus/prometheus` | `v0.314.0` | `d7598b7141418fa35be2b5ec5d0fefb634199610` | Apache-2.0 |
| `github.com/graph-gophers/graphql-go` | `v1.10.2` | `945316a96f2b309c04584a8f30f6b33422313669` | MIT |
| `github.com/graphql-go/graphql` | `v0.8.1` | release tag `v0.8.1` | MIT |

Prometheus is used as an unmodified Go module. The Apache-2.0 license text is included in this repository's [LICENSE](LICENSE); the upstream NOTICE is retained below. KQL is implemented by Argus and has no upstream parser notice in this lock manifest.

## prometheus/prometheus

Source: https://github.com/prometheus/prometheus/blob/d7598b7141418fa35be2b5ec5d0fefb634199610/NOTICE

The Prometheus systems and service monitoring server
Copyright 2012-2015 The Prometheus Authors

This product includes software developed at
SoundCloud Ltd. (https://soundcloud.com/).


The following components are included in this product:

Bootstrap
https://getbootstrap.com
Copyright 2011-2014 Twitter, Inc.
Licensed under the MIT License

bootstrap3-typeahead.js
https://github.com/bassjobsen/Bootstrap-3-Typeahead
Original written by @mdo and @fat
Copyright 2014 Bass Jobsen @bassjobsen
Licensed under the Apache License, Version 2.0

fuzzy
https://github.com/mattyork/fuzzy
Original written by @mattyork
Copyright 2012 Matt York
Licensed under the MIT License

bootstrap-datetimepicker.js
https://github.com/Eonasdan/bootstrap-datetimepicker
Copyright 2015 Jonathan Peterson (@Eonasdan)
Licensed under the MIT License

moment.js
https://github.com/moment/moment/
Copyright JS Foundation and other contributors
Licensed under the MIT License

Rickshaw
https://github.com/shutterstock/rickshaw
Copyright 2011-2014 by Shutterstock Images, LLC
See https://github.com/shutterstock/rickshaw/blob/master/LICENSE for license details

mustache.js
https://github.com/janl/mustache.js
Copyright 2009 Chris Wanstrath (Ruby)
Copyright 2010-2014 Jan Lehnardt (JavaScript)
Copyright 2010-2015 The mustache.js community
Licensed under the MIT License

jQuery
https://jquery.org
Copyright jQuery Foundation and other contributors
Licensed under the MIT License

Protocol Buffers for Go with Gadgets
https://github.com/gogo/protobuf/
Copyright (c) 2013, The GoGo Authors.
See source code for license details.

Go support for leveled logs, analogous to
https://code.google.com/p/google-glog/
Copyright 2013 Google Inc.
Licensed under the Apache License, Version 2.0

Support for streaming Protocol Buffer messages for the Go language (golang).
https://github.com/matttproud/golang_protobuf_extensions
Copyright 2013 Matt T. Proud
Licensed under the Apache License, Version 2.0

DNS library in Go
https://miek.nl/2014/august/16/go-dns-package/
Copyright 2009 The Go Authors, 2011 Miek Gieben
See https://github.com/miekg/dns/blob/master/LICENSE for license details.

LevelDB key/value database in Go
https://github.com/syndtr/goleveldb
Copyright 2012 Suryandaru Triandana
See https://github.com/syndtr/goleveldb/blob/master/LICENSE for license details.

gosnappy - a fork of code.google.com/p/snappy-go
https://github.com/syndtr/gosnappy
Copyright 2011 The Snappy-Go Authors
See https://github.com/syndtr/gosnappy/blob/master/LICENSE for license details.

go-zookeeper - Native ZooKeeper client for Go
https://github.com/samuel/go-zookeeper
Copyright (c) 2013, Samuel Stauffer <samuel@descolada.com>
See https://github.com/samuel/go-zookeeper/blob/master/LICENSE for license details.

Time series compression algorithm from Facebook's Gorilla paper
https://github.com/dgryski/go-tsz
Copyright (c) 2015,2016 Damian Gryski <damian@gryski.com>
See https://github.com/dgryski/go-tsz/blob/master/LICENSE for license details.

The Go programming language
https://go.dev/
Copyright (c) 2009 The Go Authors
See https://go.dev/LICENSE for license details.

The Codicon icon font from Microsoft
https://github.com/microsoft/vscode-codicons
Copyright (c) Microsoft Corporation and other contributors
See https://github.com/microsoft/vscode-codicons/blob/main/LICENSE for license details.

Mantine UI
https://github.com/mantinedev/mantine
Copyright (c) 2021 Vitaly Rtishchev
See https://github.com/mantinedev/mantine/blob/master/LICENSE for license details.

We also use code from a large number of npm packages. For details, see:
- https://github.com/prometheus/prometheus/blob/main/web/ui/react-app/package.json
- https://github.com/prometheus/prometheus/blob/main/web/ui/react-app/package-lock.json
- https://github.com/prometheus/prometheus/blob/main/web/ui/mantine-ui/package.json
- https://github.com/prometheus/prometheus/blob/main/web/ui/pnpm-lock.yaml
- The individual licenses of the packages bundled into the new (mantine) web UI
  are collected at build time, embedded in the Prometheus binary, and served by
  the web UI at /assets/third-party-licenses.txt.
- The licenses of the packages bundled into the old (react-app) web UI, served
  via --enable-feature=old-ui, are extracted at build time into a
  *.LICENSE.txt file that is embedded alongside the old UI's JavaScript bundle.

## graph-gophers/graphql-go

Source: https://github.com/graph-gophers/graphql-go/blob/945316a96f2b309c04584a8f30f6b33422313669/LICENSE

Copyright (c) 2016 Richard Musiol. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

## graphql-go/graphql

Source: https://github.com/graphql-go/graphql/blob/v0.8.1/LICENSE

The MIT License (MIT)

Copyright (c) 2015 Chris Ramón

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
