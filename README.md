[![Coverage Status](https://coveralls.io/repos/github/esell/deb-simple/badge.svg?branch=master)](https://coveralls.io/github/esell/deb-simple?branch=master)

# MAINTAINER WANTED

**As I am no longer using deb-simple in my day-to-day life, it has been mostly ignored as you can likely see from the issues/PRs. If you are interested in taking over and becoming the maintainer of deb-simple, please open an issue and we can start talking about transitioning it.**


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
- Supports signing package release files

# What it doesn't do:
- Create actual packages
- Mirror existing repos


# General Usage:

**This project is now using the native Go vendoring feature so you will need to build with Go >1.7 or if using 1.5/1.6 you will need to make sure `GO15VENDOREXPERIMENT` is set to `1`.**		


If you do not want to build from source you can just download a pre-built binary from the Releases section.

Fill out the conf.json file with the values you want, it should be pretty self-explanatory, then fire it up!

Once it is running POST a file to the `/upload` endpoint:

`curl -XPOST 'http://localhost:9090/upload?arch=amd64&distro=stable&section=main' -F "file=@myapp.deb"`

Or delete an existing file:

`curl -XDELETE 'http://localhost:9090/delete' -d '{"filename":"myapp.deb","distroName":"stable","arch":"amd64", "section":"main"}'`

To use your new repo you will have to add a line like this to your sources.list file:

`deb http://my-hostname:listenPort/ stable main`

`my-hostname` should be the actual hostname/IP where you are running deb-simple and `listenPort` will be whatever you set in the config. By default deb-simple puts everything into the `stable` distro and `main` section but these can be changed in the config. If you have enabled SSL you will want to swap `http` for `https`.

# Package Signing

deb-simple can sign the package release file for you, which will stop `apt-get` from complaining about insecure sources when you update. To do this you need to enable it in the config file by setting `enableSigning` to `true`, and `privateKey` to the path to your GPG signing key.

If you don't have an existing key deb-simple can help generate one for you. Run:
```
./deb-simple -k -kn "My Name" -ke "my.email@provider.com"
```

This will produce two files in the current directory: `public.key` and `private.key`. I suggest putting `public.key` in the repository root somewhere so it can be downloaded by clients that need it, and putting `private.key` somewhere
relatively secure on the file system.

To add your new key on a client run the following command:
```
wget -qO - http://my-hostname:listenPort/public.key | sudo apt-key add -
```

This uses Go's native `openpgp` library, so key support is cross platform, and doesn't require or interact with any
existing keyring on the system.

# Using API keys:

deb-simple supports the idea of an API key to limit who can upload and delete packages. To use the API keys feature you first need to enable it in the config file by setting `enableAPIKeys` to `true`. Once that is done you'll need to generate at least one API key. To do that just run `deb-simpled -g` and an API key will be printed to stdout.

Now that you have a key you'll need to include it in your `POST` and `DELETE` requests by simply adding on the `key` URL parameter. An example for an upload might look like:

`curl -XPOST 'http://localhost:9090/upload?arch=amd64&distro=stable&section=main&key=MY_BIG_API_KEY' -F "file=@myapp.deb"`

A delete would look like:

`curl -XDELETE 'http://localhost:9090/delete?key=MY_BIG_API_KEY' -d '{"filename":"myapp.deb","distroName":"stable","arch":"amd64", "section":"main"}'`

If you want an automatable service which builds you packages, either manualy or via CI/CD, checkout [debpkg](https://github.com/xor-gate/debpkg),
which makes it very easy to create complex packages with almost no work.  

If you want to continuous deliver created packages to deb-simple server, it is not recommended to place the key
somewhere others could find it. You can use [deb-simple-cd-helper](https://github.com/paulkramme/deb-simple-cd-help),
which allows you to place a plaintext file with the api key somewhere on your build server without the need to expose it.

# Directory Watching
By default `deb-simple` will watch the directories it creates for any new files and rebuild the repository accordingly. This means that you don't have to use the HTTP interface to upload new packages if you have a different build system - any method of getting them onto the server will work.

This function means that there is a delay between a package being uploaded / created and it being availble for installation, as the repository rebuild happens asynchronously. This can result in errors like `Hash Sum Mismatch` from `apt install` processes if you happen to update in the middle of a rebuild.

You can disable the watching behaviour by setting `enableDirectoryWatching=false` in the `conf.json` file. In this case the repository will be rebuilt as part of the HTTP file upload process, so once your CI build / `curl` upload has completed the package will be ready for installation.

# Do you use this?

If you use deb-simple somewhere I'd love to hear about it! Make a PR to add your company/group/cult :)

- [ASE](https://www.aseit.com.au) ASE uses deb-simple to serve their FLUID application resources to devices in the wild :)

# Contributors

- [icholy](https://github.com/icholy)
- [dig412](https://github.com/dig412)
- [tystuyfzand](https://github.com/tystuyfzand)
- [kshvakov](https://github.com/kshvakov)
- [pkramme](https://github.com/pkramme)
- [alexanderturner](https://githib.com/alexanderturner)


# License:

[MIT](LICENSE.txt) so go crazy
