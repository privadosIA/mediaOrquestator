# The DiffRhythm2 music engine as a small HTTP service (see README.md).
# The weights are NOT in the image: they are mounted at /ckpt (read-only for the service, writable for fetch.py).
FROM python:3.11-slim-bookworm

# The commit of https://github.com/ASLP-lab/DiffRhythm2 the service was made and tested with.
ARG DR2_COMMIT=7804f821b797b4f276090e1a9dcd37e97d9915d5
ARG DIFFRHYTHM2_VERSION=1

# espeak-ng: the phoneme front end; the compilers and cmake: pyopenjtalk is built from source; ffmpeg and
# libsndfile: the audio libraries read and write files with them.
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates espeak-ng ffmpeg libsndfile1 build-essential cmake \
    && rm -rf /var/lib/apt/lists/*

# PyTorch with the CUDA 12.8 wheels: they run on the RTX 4070 and on Blackwell cards such as the RTX 5060 Ti.
# The CUDA libraries come inside the wheels; the driver comes from the NVIDIA container runtime.
RUN pip install --no-cache-dir torch==2.7.0 torchaudio==2.7.0 --index-url https://download.pytorch.org/whl/cu128

RUN git clone https://github.com/ASLP-lab/DiffRhythm2.git /opt/dr2 \
    && cd /opt/dr2 && git checkout "${DR2_COMMIT}"

# The repository's own requirements, without torch (installed above from the CUDA wheels).
RUN grep -v -E '^(torch|torchaudio)(==|$)' /opt/dr2/requirements.txt > /tmp/requirements.txt \
    && pip install --no-cache-dir -r /tmp/requirements.txt

COPY server.py fetch.py /opt/

ENV DIFFRHYTHM2_CKPT=/ckpt \
    DIFFRHYTHM2_VERSION=${DIFFRHYTHM2_VERSION} \
    DIFFRHYTHM2_UPSTREAM=/opt/dr2 \
    PYTHONPATH=/opt/dr2 \
    PYTHONUNBUFFERED=1
WORKDIR /opt/dr2
# Not root: the service only reads /ckpt and writes to /tmp and its own home (the compile and model caches). The
# application starts the containers with its own --user, which takes precedence.
RUN useradd --create-home --uid 10001 app
USER app
EXPOSE 8788
CMD ["python", "/opt/server.py"]
