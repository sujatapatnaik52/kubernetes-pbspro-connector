#PBS -joe -o localhost:/tmp
sleep 30
while :
do
       	docker ps | grep $PODNAME
        if [ $? -ne 0 ]; then
                exit 0
        else
                sleep 5
        fi
done
