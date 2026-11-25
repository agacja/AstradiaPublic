// Only this public service is offered as source. No parent directory traversal.
fn main() -> std::io::Result<()> {
    let output = std::path::PathBuf::from(
        std::env::var_os("OUT_DIR").ok_or_else(|| std::io::Error::other("OUT_DIR is not set"))?,
    );
    let gzip = flate2::write::GzEncoder::new(
        std::fs::File::create(output.join("source.tar.gz"))?,
        flate2::Compression::default(),
    );
    let mut archive = tar::Builder::new(gzip);
    for file in [
        "Cargo.toml",
        "Cargo.lock",
        "clippy.toml",
        "build.rs",
        "src/main.rs",
        "src/protocol.rs",
        "LICENSE",
        "SWISS-LICENSE",
        "PROTOCOL-LICENSE",
        "README.md",
        "Dockerfile",
    ] {
        println!("cargo:rerun-if-changed={file}");
        archive.append_path_with_name(file, format!("astradia-ephemeris/{file}"))?;
    }
    for entry in std::fs::read_dir("data/ephemeris")? {
        let entry = entry?;
        if entry.file_type()?.is_file() {
            println!("cargo:rerun-if-changed={}", entry.path().display());
            archive.append_path_with_name(
                entry.path(),
                std::path::Path::new("astradia-ephemeris/data/ephemeris").join(entry.file_name()),
            )?;
        }
    }
    archive.into_inner()?.finish()?;
    Ok(())
}
