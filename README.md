# deb-simple (get it? dead simple.. deb simple...)

A lightweight, bare-bones apt repository server. 

# Purpose

This project came from a need I had to be able to serve up already created deb packages without a lot of fuss. Most of the existing solutions 
I found were either geared at mirroring existing "official" repos or for providing your packages to the public. My need was just something that 
I could use internally to install already built deb packages via apt-get. I didn't care about change files, signed packages, etc. Since this was 
to be used in a CI pipeline it had to support remote uploads and be able to update the package list after each upload.

# What it does:

- Supports multiple versions of packages 
- Supports uploading via HTTP POST requests
- Does NOT require a changes file
- Supports uploads from various locations without corrupting the repo


# What it doesn't do:
- Create actual packages
- Mirror existing repos

# TODO:

- [x] Remove dpkg-scanpackages from the equation
- [x] Remove gzip from the equation
- [x] Actually handle multi-arch repos
- [ ] Support SSL
- [ ] Add delete ability

# Usage:

Install using `go get`. Fill out the conf.json file with the values you want, it should be pretty self-explanatory, then fire it up!

Once it is running POST a file to the `/upload` endpoint:
`curl -XPOST 'http://localhost:9090/upload' -F "file=@myapp.deb" -F "arch=amd64"`

#License:

[MIT](LICENSE.txt) so go crazy. Would appreciate PRs for anything cool you add though :)
