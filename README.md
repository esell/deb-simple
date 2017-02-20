[![Build Status](https://drone.esheavyindustries.com/api/badges/esell/deb-simple/status.svg)](https://drone.esheavyindustries.com/esell/deb-simple)
[![Coverage](http://esheavyindustries.com:8080/display?repo=debsimple_git)](http://esheavyindustries.com:8080/display?repo=debsimple_git)


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
- Supports API keys to protect who can upload/delete packages


# What it doesn't do:
- Create actual packages
- Mirror existing repos


# General Usage:

__This project is now using the native Go vendoring feature so you will need to build with Go >1.7 or if using 1.5/1.6 you will need to make sure `GO15VENDOREXPERIMENT` is set to `1`.__

If you do not want to build from source you can just download a pre-built binary from the Releases section.

Fill out the conf.json file with the values you want, it should be pretty self-explanatory, then fire it up!

Once it is running POST a file to the `/upload` endpoint:

`curl -XPOST 'http://localhost:9090/upload?arch=amd64&distro=stable&section=main' -F "file=@myapp.deb"`

Or delete an existing file:

`curl -XDELETE 'http://localhost:9090/delete' -d '{"filename":"myapp.deb","distroName":"stable","arch":"amd64", "section":"main"}'`

To use your new repo you will have to add a line like this to your sources.list file:

`deb http://my-hostname:listenPort/ stable main`

`my-hostname` should be the actual hostname/IP where you are running deb-simple and `listenPort` will be whatever you set in the config. By default deb-simple puts everything into the `stable` distro and `main` section but these can be changed in the config. If you have enabled SSL you will want to swap `http` for `https`.


# Using API keys:

deb-simple supports the idea of an API key to limit who can upload and delete packages. To use the API keys feature you first need to enable it in the config file by setting `enableAPIKeys` to `true`. Once that is done you'll need to generate at least one API key. To do that just run `deb-simpled -g` and an API key will be printed to stdout. 

Now that you have a key you'll need to include it in your `POST` and `DELETE` requests by simply adding on the `key` URL parameter. An example for an upload might look like:

`curl -XPOST 'http://localhost:9090/upload?arch=amd64&distro=stable&section=main&key=MY_BIG_API_KEY' -F "file=@myapp.deb"`

A delete would look like:

`curl -XDELETE 'http://localhost:9090/delete?key=MY_BIG_API_KEY' -d '{"filename":"myapp.deb","distroName":"stable","arch":"amd64", "section":"main"}'`


#License:

[MIT](LICENSE.txt) so go crazy. Would appreciate PRs for anything cool you add though :)
