# sourceafis-sidecar: SourceAFIS for Java behind a plain JDK HTTP server.
# Dependencies come from Maven Central via fetch-deps.sh (no Maven needed).
FROM eclipse-temurin:21-jdk AS build
WORKDIR /src
RUN command -v curl >/dev/null || (apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*)
COPY fetch-deps.sh .
RUN ./fetch-deps.sh
COPY Sidecar.java .
RUN javac -cp 'lib/*' -d classes Sidecar.java

FROM eclipse-temurin:21-jre
WORKDIR /app
COPY --from=build /src/lib lib
COPY --from=build /src/classes classes
ENV PORT=8090
EXPOSE 8090
CMD ["java", "-cp", "classes:lib/*", "Sidecar"]
