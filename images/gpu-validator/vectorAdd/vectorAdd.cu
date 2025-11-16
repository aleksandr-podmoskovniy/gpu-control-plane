#include <cuda_runtime.h>
#include <cmath>
#include <cstdio>
#include <cstdlib>

#define CHECK_CUDA(call)                                                   \
  do {                                                                     \
    cudaError_t err = call;                                                \
    if (err != cudaSuccess) {                                              \
      fprintf(stderr, "CUDA error %s:%d: %s\n", __FILE__, __LINE__,        \
              cudaGetErrorString(err));                                    \
      return EXIT_FAILURE;                                                 \
    }                                                                      \
  } while (0)

__global__ void vectorAddKernel(const float* A, const float* B, float* C,
                                int numElements) {
  int i = blockDim.x * blockIdx.x + threadIdx.x;
  if (i < numElements) {
    C[i] = A[i] + B[i];
  }
}

int main() {
  const int numElements = 1 << 20;
  const size_t size = numElements * sizeof(float);

  float* h_A = static_cast<float*>(malloc(size));
  float* h_B = static_cast<float*>(malloc(size));
  float* h_C = static_cast<float*>(malloc(size));

  if (!h_A || !h_B || !h_C) {
    fprintf(stderr, "Host memory allocation failed\n");
    free(h_A);
    free(h_B);
    free(h_C);
    return EXIT_FAILURE;
  }

  for (int i = 0; i < numElements; ++i) {
    h_A[i] = static_cast<float>(i) * 0.5f;
    h_B[i] = static_cast<float>(i) * 2.0f;
  }

  float *d_A = nullptr, *d_B = nullptr, *d_C = nullptr;
  CHECK_CUDA(cudaMalloc(&d_A, size));
  CHECK_CUDA(cudaMalloc(&d_B, size));
  CHECK_CUDA(cudaMalloc(&d_C, size));

  CHECK_CUDA(cudaMemcpy(d_A, h_A, size, cudaMemcpyHostToDevice));
  CHECK_CUDA(cudaMemcpy(d_B, h_B, size, cudaMemcpyHostToDevice));

  const int threadsPerBlock = 256;
  const int blocksPerGrid = (numElements + threadsPerBlock - 1) / threadsPerBlock;
  vectorAddKernel<<<blocksPerGrid, threadsPerBlock>>>(d_A, d_B, d_C, numElements);
  CHECK_CUDA(cudaGetLastError());
  CHECK_CUDA(cudaDeviceSynchronize());

  CHECK_CUDA(cudaMemcpy(h_C, d_C, size, cudaMemcpyDeviceToHost));

  bool success = true;
  for (int i = 0; i < numElements; ++i) {
    const float expected = h_A[i] + h_B[i];
    if (fabs(h_C[i] - expected) > 1e-5f) {
      fprintf(stderr, "Validation failure at index %d: got %f expected %f\n", i,
              h_C[i], expected);
      success = false;
      break;
    }
  }

  cudaFree(d_A);
  cudaFree(d_B);
  cudaFree(d_C);
  free(h_A);
  free(h_B);
  free(h_C);

  if (!success) {
    return EXIT_FAILURE;
  }

  printf("CUDA vectorAdd validation succeeded\n");
  return EXIT_SUCCESS;
}
