FROM ubuntu:24.04
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends maven openjdk-21-jdk-headless
RUN echo "MAVEN_OPTS=$MAVEN_OPTS"
RUN mvn -B -q dependency:get -Dartifact=junit:junit:4.13.2 && echo MAVEN-FETCH-OK
RUN ls ~/.m2/repository/junit/junit/4.13.2/ > /tmp/m2.txt && warden-io post /tmp/m2.txt
