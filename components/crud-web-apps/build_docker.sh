docker build -t jupyter . --no-cache
docker images
docker tag jupyter cguaicadmin/jupyter:v0.2.0x
docker push cguaicadmin/jupyter:v0.2.0x

