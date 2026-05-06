FROM ubuntu:20.04
WORKDIR /work
COPY . /work

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHON_BIN_PATH=/usr/bin/python3 \
    PYTHON_LIB_PATH=/usr/local/lib/python3.8/dist-packages \
    TF_NEED_CUDA=1 \
    CUDA_TOOLKIT_PATH=/usr/local/cuda-11.8 \
    TF_CUDNN_VERSION=8 \
    CUDANN_INSTALL_PATH=/usr/include \
    TF_CUDA_COMPUTE_CAPABILITIES=3.5,6.0,7.0 \
    TF_CUDA_VERSION=11 \
    GCC_HOST_COMPILER_PATH=/usr/bin/gcc \
    CC_OPT_FLAGS="--config=cuda" \
    TF_NEED_TENSORRT=0 \
    TF_NEED_ROCM=0 \
    TMP=/tmp \
    TMP_DIR=/tmp \
    TF_SET_ANDROID_WORKSPACE=0

RUN apt update && \
    apt install -y lsb-release software-properties-common wget && \
    wget -O llvm.sh https://apt.llvm.org/llvm.sh && \
    chmod +x llvm.sh && \
    ./llvm.sh 16 && \
    wget -qO - https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/3bf863cc.pub | apt-key add - && \
    apt-add-repository "deb http://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64 /" && \
    apt update && \
    apt install -y \
        cuda-toolkit-11-8 \
        "libcudnn8-dev=8.6.0.163-1+cuda11.8" \
        "libcudnn8=8.6.0.163-1+cuda11.8" \
    && \
    apt update && \
    apt install -y python3-dev python3-pip git patchelf && \
    ln -s /usr/bin/python3 /usr/bin/python && \
    pip install -U --user pip numpy wheel packaging requests opt_einsum && \
    pip install -U --user keras_preprocessing --no-deps && \
    wget -O /usr/bin/bazel "https://github.com/bazelbuild/bazelisk/releases/download/v1.17.0/bazelisk-linux-amd64" && \
    chmod +x /usr/bin/bazel && \
    wget -O tensorflow.tar.gz "https://github.com/tensorflow/tensorflow/archive/refs/tags/v2.13.0.tar.gz" && \
    tar xaf tensorflow.tar.gz && \
    cd "tensorflow-2.13.0" && \
    ./configure && \
    bazel build -c opt --config=cuda --verbose_failures --action_env=PIP_CERT=${PIP_CERT} //tensorflow/tools/pip_package:build_pip_package && \
    ./bazel-bin/tensorflow/tools/pip_package/build_pip_package /tmp/tensorflow_pkg && \
    pip install /tmp/tensorflow_pkg/tensorflow-*.whl

    # wget -O cuda.deb "https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/cuda-11-8_11.8.0-1_amd64.deb" && \
    # wget -O cuda-runtime.deb "https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/cuda-runtime-11-8_11.8.0-1_amd64.deb" && \
    # wget -O cudnn.deb "https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/libcudnn8_8.6.0.163-1+cuda11.8_amd64.deb" && \
    # wget -O cudnn-dev.deb "https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/libcudnn8-dev_8.6.0.163-1+cuda11.8_amd64.deb" && \
    # apt install -y ./cuda.deb ./cudnn.deb ./cudnn-dev.deb && \

    # wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/cuda-keyring_1.1-1_all.deb && \
    # dpkg -i cuda-keyring_1.1-1_all.deb && \

    # cp .warden/ca.crt /etc/ssl/certs/ca-certificates.crt && \
