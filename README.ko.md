# Certree

**배포 후 실제 서비스 인증서까지 검증하는 자체 호스팅 TLS 인증서 관리 도구입니다.**

외부에서 발급받은 인증서를 가져와 검토·승인하고 Nginx에 적용한 뒤, 실제 TLS
연결에서 같은 인증서가 제공되는지 확인합니다. 인증서·서버 관계, 만료 시점과
배포 결과를 한 화면에서 관리하는 것이 목표입니다.

[English](README.md) · [시작 가이드](docs/GETTING_STARTED.ko.md) ·
[운영](docs/OPERATIONS.md) · [기여](CONTRIBUTING.md) · [보안](SECURITY.md)

현재는 **v0.1.0 후보 버전**이며 Linux 에이전트는 개발 프리뷰입니다.
Linux race·실제 Nginx 에이전트·운영 이미지·Docker 통합 검사는
[CI에서 통과했습니다](https://github.com/polarbear138/certree/actions/runs/40154857361).
실제 systemd 설치와 릴리스 보안 검토는 별도 검증이 필요합니다.
구현 여부와 실행된 검증 범위는 [검증 보고서](docs/VERIFICATION.md)에서 구분합니다.

## 어떤 문제를 해결하나요?

인증서를 발급받는 일과 모든 서버에 정확히 적용하는 일은 별개입니다.
인증서 파일 복사나 Nginx 재로드가 성공해도 실제 서비스에서는 이전 인증서가
계속 제공될 수 있습니다.

Certree는 신뢰할 수 있는 TLS 연결, 호스트명 검증, 승인된 인증서의 SHA-256
지문 일치를 확인해야 배포를 성공으로 기록합니다. 실패나 복구 결과도 이력에
남깁니다. 외부 발급 인증서를 운영하는 Linux/Nginx 관리자가 주요 대상입니다.

## 주요 기능

| 기능 | 현재 동작 |
| --- | --- |
| Tree Explorer | 조직 → 환경 → 인증서 → 서버의 서비스 엔드포인트 관계 탐색, 검색·상태 필터·키보드 이동 |
| 인증서 검증 | PEM/DER/PKCS#12/제한된 ZIP 처리, 개인키 일치·유효기간·호스트명·신뢰 체인 검사 |
| 버전과 승인 | 변경 불가 버전 보관, 배포 전 명시적 승인 |
| 배포 경로 | 호스트 키를 고정한 SSH/SFTP 또는 외부로 HTTPS 연결하는 Linux 에이전트 |
| 일괄 배포 | 체크박스로 대상 선택, 승인된 구체적인 버전 미리보기, 서버별 순차 대기열, 대상별 결과 |
| 만료 표시 | 마지막으로 적용·검증된 인증서의 남은 일수와 프로그레스바, 서버별 가장 빠른 만료 요약 |
| History | Verification 기본 탭과 Deployments 탭 분리, 서버별 필터와 작업 상세 |
| 수집과 모니터링 | TLS IMAP 첨부 수집·중복 방지·관리자 승인, 알려진 활성 버전을 5분마다 검증 |
| 업데이트 | 기존 DB·암호화 키·CA 마운트·Compose 프로젝트를 보존하고 앱만 재빌드 |

5분 주기로 생성되는 작업은 **상태 검증**입니다. 실제 배포는 관리자의 배포
요청으로 실행합니다. 서버 만료 바도 최신 업로드 파일이 아닌 마지막으로
적용·검증된 버전을 기준으로 합니다.

## 새로 설치하기

Docker Engine과 Compose v2, `go.mod`에 지정된 Go 버전이 필요합니다.

```sh
git clone https://github.com/polarbear138/certree.git
cd certree
go run ./cmd/setup
docker compose up -d --build
docker compose ps
```

설치한 장비에서 `https://localhost:8443`으로 접속합니다. `.env`에 생성된 초기
계정으로 로그인하고 Settings에서 비밀번호를 바꾸세요. `.env`의 암호화 키는
기존 데이터를 복호화할 때 필요하므로 별도로 안전하게 보관해야 합니다.

기본값은 `127.0.0.1:8443` 바인딩과 Caddy 내부 CA입니다. **서버 밖 PC에서
접속하려면** 공개할 관리 인터페이스·접속 주소·Caddy 설정·인증서 신뢰를
함께 맞춰야 합니다. [한국어 시작 가이드](docs/GETTING_STARTED.ko.md)를 따르세요.

## 기존 환경 업데이트하기

처음에는 새 소스를 별도 폴더에 풀고 그 소스 루트에서 기존 앱 컨테이너를
지정합니다. 기존 설치 폴더와 CA 파일은 보존합니다.

```sh
sh scripts/update.sh --container certree-certree-1
```

기존 두 서버 테스트 환경의 앱 이름은 `certree-two-server-test-certree-1`일 수
있습니다. 실제 컨테이너 이름을 사용하세요. 저장된 설치 프로필이 있는 Git
체크아웃에서는 이후 다음 명령으로 소스 갱신과 앱 재빌드를 처리합니다.

```sh
sh scripts/update.sh --pull
```

`git pull`만으로 실행 중인 Docker 이미지가 바뀌지는 않습니다. 로컬에서만
수정한 코드는 먼저 원격 저장소에 커밋·푸시되어야 받을 수 있습니다.
[업데이트 가이드](docs/UPDATING.md)에 ZIP에서 Git으로 옮기는 과정과 보관할
설정 경로를 설명했습니다.

## 지원 범위와 한계

현재 배포 어댑터는 Nginx입니다. 인증서 발급, 다른 웹 서버 어댑터, RBAC,
HA, OCSP/CRL 검사와 자동 키 회전은 제공하지 않습니다. DB 하나에 관리
프로세스 하나만 실행할 수 있습니다. 배포·복구는 관리자와 관리 호스트를
신뢰하며, 인증서와 키의 원자적 교체는 파일 각각에 적용됩니다.

OpenAI API 키는 제품 실행에 필요하지 않습니다. Codex는 개발·테스트·문서화
작업을 보조하며, 인증서 승인과 실행 판단은 애플리케이션의 명시적 검사로
처리합니다. 보안 감사나 광범위한 실사용 도입 실적을 주장하지 않습니다.

## 기여와 검증

Linux 통합 테스트, 복구 회귀 테스트, 설치 문서, 접근성 개선에 기여할 수
있습니다. [기여 가이드](CONTRIBUTING.md), [유지관리 가이드](docs/MAINTAINING.md),
[로드맵](ROADMAP.md)을 참고하세요. CI 정의와 실제 성공 결과는 구분합니다.
일반 오류는 [Issues](https://github.com/polarbear138/certree/issues), 보안 문제는
[보안 정책](SECURITY.md)의 비공개 경로를 이용하세요.

## 라이선스

[Apache License 2.0](LICENSE). [NOTICE](NOTICE)와
[서드파티 고지](docs/THIRD_PARTY_NOTICES.md)를 함께 확인하세요.
