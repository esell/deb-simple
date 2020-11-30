FROM alpine:latest

# Set the Current Working Directory inside the container
WORKDIR $GOPATH/src/github.com/esell/deb-simple

# Copy everything from the current directory to the PWD (Present Working Directory) inside the container
COPY deb-simple /

# This container exposes port 9090 to the outside world
EXPOSE 9090

# Run the executable
CMD ["/deb-simple -v"]
