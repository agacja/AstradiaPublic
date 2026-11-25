FROM rust:1.94-bookworm AS build
WORKDIR /build
COPY Cargo.toml Cargo.lock build.rs clippy.toml LICENSE SWISS-LICENSE PROTOCOL-LICENSE README.md Dockerfile ./
COPY src ./src
COPY data ./data
RUN cargo build --release --locked
FROM debian:bookworm-slim
RUN groupadd --system ephemeris && useradd --system --gid ephemeris ephemeris
WORKDIR /app
COPY --from=build /build/target/release/astradia-ephemeris /app/ephemeris
COPY data/ephemeris /app/data
ENV EPHEMERIS_BIND=0.0.0.0:8091 ASTRADIA_EPHEMERIS_PATH=/app/data
USER ephemeris
EXPOSE 8091
ENTRYPOINT ["/app/ephemeris"]
