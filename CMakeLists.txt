cmake_minimum_required(VERSION 3.10)
project(opus_lab C)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -Wall -Wextra -O2")

# Opus 库路径
set(OPUS_INSTALL_DIR "${CMAKE_SOURCE_DIR}/opus-install")
set(OPUS_INCLUDE_DIR "${OPUS_INSTALL_DIR}/include")
set(OPUS_LIB_DIR     "${OPUS_INSTALL_DIR}/lib")

include_directories(
    offline_validation/src
    ${OPUS_INCLUDE_DIR}
)
link_directories(${OPUS_LIB_DIR})

# ---- opus_sim: 离线仿真工具 ----
add_executable(opus_sim offline_validation/src/opus_sim.c)
target_link_libraries(opus_sim opus m)

# ---- opus_sender: UDP发送端 ----
add_executable(opus_sender offline_validation/src/sender.c)
target_link_libraries(opus_sender opus m)

# ---- opus_receiver: UDP接收端 ----
add_executable(opus_receiver offline_validation/src/receiver.c)
target_link_libraries(opus_receiver opus m)

# 编译后提示
message(STATUS "")
message(STATUS "构建目标:")
message(STATUS "  opus_sim     - 离线仿真工具 (LBRR/DRED/PLC)")
message(STATUS "  opus_sender  - UDP实时发送端")
message(STATUS "  opus_receiver- UDP实时接收端")
message(STATUS "")
message(STATUS "Opus库路径: ${OPUS_LIB_DIR}")
message(STATUS "")
