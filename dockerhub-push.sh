docker build -t pooh64/csif-plugin -f Dockerfile-plugin .
docker push pooh64/csif-plugin
docker build -t pooh64/csif-filter -f Dockerfile-filter .
docker push pooh64/csif-filter
