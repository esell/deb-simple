[![Build Status](https://travis-ci.org/esell/deb-simple.svg?branch=master)](https://travis-ci.org/esell/deb-simple)
[![Coverage Status](https://coveralls.io/repos/github/esell/deb-simple/badge.svg?branch=master)](https://coveralls.io/github/esell/deb-simple?branch=master)


# deb-simple (get it? dead simple.. deb simple...)

A lightweight, bare-bones apt repository server. 

# Purpose

This project came from a need I had to be able to serve up already created deb packages without a lot of fuss. Most of the existing solutions 
I found were either geared at mirroring existing "official" repos or for providing your packages to the public. My need was just something that 
I could use internally to install already built deb packages via apt-get. I didn't care about change files, signed packages, etc. Since this was 
to be used in a CI pipeline it had to support remote uploads and be able to update the package list after each upload.

# What it does:

- Supports multiple versions of packages 
- Supports multi-arch repos (i386, amd64, custom, etc)
- Supports uploading via HTTP/HTTPS POST requests
- Supports removing packages via HTTP/HTTPS DELETE requests
- Does NOT require a changes file
- Supports uploads from various locations without corrupting the repo


# What it doesn't do:
- Create actual packages
- Mirror existing repos


# Usage:

You'll need [GB](https://getgb.io/) to build so install that first. Clone this repo and then do a `gb build` from inside of it, your executable will be in `bin/`. 

If you do not want to build from source you can just download a pre-built binary from the Releases section.

Fill out the conf.json file with the values you want, it should be pretty self-explanatory, then fire it up!

Once it is running POST a file to the `/upload` endpoint:

`curl -XPOST 'http://localhost:9090/upload?arch=amd64&distro=stable' -F "file=@myapp.deb"`

Or delete an existing file:

`curl -XDELETE 'http://localhost:9090/delete' -d '{"filename":"myapp.deb","distroName":"stable","arch":"amd64"}'`

To use your new repo you will have to add a line like this to your sources.list file:

`deb http://my-hostname:listenPort/ stable main`

`my-hostname` should be the actual hostname/IP where you are running deb-simple and `listenPort` will be whatever you set in the config. By default deb-simple puts everything into the `stable` distro and `main` section. If you have enabled SSL you will want to swap `http` for `https`.


#License:

[MIT](LICENSE.txt) so go crazy. Would appreciate PRs for anything cool you add though :)
