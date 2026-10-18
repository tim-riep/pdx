# EU DSS Demo-Webapp als Prüfinstanz für PAdES, nur für Tests und lokal
# gebaut (LGPL-2.1, https://github.com/esig/dss-demonstrations). Kontext ist
# der Quellstand von Tag und Commit aus dssval.go, build.sh holt ihn.
# Entspricht dem Dockerfile des Projekts, mit Basisimages per Digest und
# entpacktem WAR, damit die Vertrauensanker beim Start eingebunden werden.
FROM docker.io/library/maven:3.9.16-eclipse-temurin-26@sha256:b2c1ad85954592f9928e84327c65201f308ad9b5d8ed7d5b823717c97bf23fbb AS build
WORKDIR /src
COPY . .
RUN mvn -q package -pl dss-standalone-app,dss-standalone-app-package,dss-demo-webapp -P quick,linux -DskipTests

FROM docker.io/library/tomcat:11.0.27-jdk25-temurin@sha256:fc6089642155a7cdad0cf118cf1fcca74a2f89e90169bdf913b2e38e603c0b47
RUN rm -rf /usr/local/tomcat/webapps/*
COPY --from=build /src/dss-demo-webapp/target/dss-demo-webapp-*.war /tmp/root.war
RUN mkdir /usr/local/tomcat/webapps/ROOT && cd /usr/local/tomcat/webapps/ROOT \
 && jar xf /tmp/root.war && rm /tmp/root.war \
 && mkdir WEB-INF/classes/pdx
# Ohne EU-Vertrauenslisten (offline), Vertrauensanker aus pdx/trust.p12,
# das der Runner je Testlauf einbindet.
RUN printf '%s\n' \
  'cron.tl.loader.enable = false' \
  'tl.loader.ades.enabled = false' \
  'trusted.source.keystore.type = PKCS12' \
  'trusted.source.keystore.filename = pdx/trust.p12' \
  'trusted.source.keystore.password = pdx' \
  'dataloader.connection.timeout = 3000' \
  'cxf.debug = false' \
  > /usr/local/tomcat/webapps/ROOT/WEB-INF/classes/dss-custom.properties
EXPOSE 8080
