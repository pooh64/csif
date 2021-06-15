docker build -t pooh64/csif-plugin-csivol -f Dockerfile-plugin .
docker push pooh64/csif-plugin-csivol
docker build -t pooh64/csif-filter-csivol -f Dockerfile-filter .
docker push pooh64/csif-filter-csivol