#!/bin/bash

items=(
	"atom e529f355d64cebe2d47e18500909f5a1e24b6cb5bdb329612ef078acb38217a2"
	"skype 0d160ea7344c5b42c6c6651e2b97340076dec9ce1945f6889139f36cc2b3015c"
	"slack bb5a400cfd8d35101558e0dd48ea71110277af233e6184d1251fc1f5576b9f11"
	"spotify 7f43e55cd9868899a9faa27a38ec8792a7c950230c4fbdc797c5447ce49ec611"
	"cheese 1f8cb78f8fdf2c4fbcc421319d6b4f45229faa33108c22ed9cbd4773dd39bb32"
)

for i in "${items[@]}"
do
	item=($i)
	notary add-checksum docker.io/conman/apps "${item[0]}" "${item[1]}" 123 --custom=${item[0]}.json
done
