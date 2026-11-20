# The HeartMuLa music engine as a small HTTP service (see README.md).
# The weights are NOT in the image: they are mounted read-only at /ckpt.
FROM python:3.11-slim-bookworm

# The commit of https://github.com/HeartMuLa/heartlib the service was made and tested with.
ARG HEARTLIB_COMMIT=5858ca8f1ffe58d62be7c007ea55f1033c5ca456
ARG HEARTMULA_VERSION=1

# gcc: the library compiles parts of the model with torch.compile, which needs a C compiler.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg git ca-certificates libsndfile1 gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# PyTorch with the CUDA 12.8 wheels: they run on the RTX 4070 and on Blackwell cards such as the RTX 5060 Ti.
# The CUDA libraries come inside the wheels; the driver comes from the NVIDIA container runtime.
RUN pip install --no-cache-dir "torch>=2.4,<2.11" "torchaudio>=2.4,<2.11" "torchvision>=0.19,<0.26" \
    --index-url https://download.pytorch.org/whl/cu128

RUN git clone https://github.com/HeartMuLa/heartlib.git /opt/heartlib \
    && cd /opt/heartlib && git checkout "${HEARTLIB_COMMIT}"

# Two changes to the library (see the patch): the attention cache sized to the song, and the audio saved
# with soundfile instead of torchaudio.
COPY heartlib-memory-and-save.patch /opt/heartlib-memory-and-save.patch
RUN cd /opt/heartlib && git apply /opt/heartlib-memory-and-save.patch && pip install --no-cache-dir -e .

COPY server.py /opt/server.py
COPY fetch.py /opt/fetch.py

ENV HEARTMULA_CKPT=/ckpt \
    HEARTMULA_VERSION=${HEARTMULA_VERSION} \
    PYTHONUNBUFFERED=1
# Not root: the service only reads /ckpt and writes to /tmp and its own home (the compile and model caches). The
# application starts the containers with its own --user, which takes precedence.
RUN useradd --create-home --uid 10001 app
USER app
EXPOSE 8787
CMD ["python", "/opt/server.py"]
